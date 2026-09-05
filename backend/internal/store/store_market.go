package store

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// 目录前缀 → 部门 ID（优先匹配双段前缀）
var deptPrefixMap = map[string]string{
	// 双段前缀（优先）
	"paid-media":         "sales-marketing",
	"supply-chain":       "supply-chain",
	"project-management": "project-management",
	"project-manager":    "project-management",
	"technical-artist":   "creative-tech",
	"game-development":   "creative-tech",
	"spatial-computing":  "spatial-computing",
	// 单段前缀
	"academic":    "academic",
	"engineering": "engineering",
	"design":      "design",
	"marketing":   "marketing",
	"product":     "product",
	"testing":     "testing",
	"support":     "support",
	"specialized": "specialized",
	"agents":      "specialized",
	"prompt":      "specialized",
	"game":        "creative-tech",
	"godot":       "creative-tech",
	"unity":       "creative-tech",
	"unreal":      "creative-tech",
	"roblox":      "creative-tech",
	"blender":     "creative-tech",
	"xr":          "creative-tech",
	"visionos":    "creative-tech",
	"level":       "creative-tech",
	"narrative":   "creative-tech",
	"finance":     "finance",
	"hr":          "hr",
	"legal":       "legal",
	"sales":       "sales-marketing",
	"company":     "company",
	"security":    "security",
	"strategy":    "strategy",
	"gis":         "gis",
}

// 部门 ID → 中文名称
var deptNameMap = map[string]string{
	"engineering":        "工程部",
	"marketing":          "营销部",
	"design":             "设计部",
	"product":            "产品部",
	"project-management": "项目管理部",
	"testing":            "测试部",
	"support":            "支持部",
	"specialized":        "专项部",
	"creative-tech":      "创意技术部",
	"finance":            "金融部",
	"hr":                 "HR部",
	"legal":              "法务部",
	"sales-marketing":    "销售与营销部",
	"supply-chain":       "供应链部",
	"academic":           "学术部",
	"company":            "公司经营部",
	"security":           "安全部",
	"strategy":           "战略部",
	"gis":                "地理信息部",
	"spatial-computing":  "空间计算部",
	"other":              "其他",
}

// 部门显示顺序
var deptOrder = []string{
	"company",
	"engineering",
	"marketing",
	"design",
	"product",
	"project-management",
	"testing",
	"support",
	"specialized",
	"creative-tech",
	"finance",
	"hr",
	"legal",
	"sales-marketing",
	"supply-chain",
	"academic",
	"security",
	"strategy",
	"gis",
	"spatial-computing",
	"other",
}

// ── 独立函数：供生成脚本调用 ──

// BuildRolesIndex 扫描 rolesDir 目录，构建并返回角色索引。
// filePathPrefix 是写入 JSON 中 files 字段的路径前缀（相对于后端工作目录，如 "data/roles"）。
func BuildRolesIndex(rolesDir, filePathPrefix string) (*model.RolesIndex, error) {
	entries, err := os.ReadDir(rolesDir)
	if err != nil {
		return nil, fmt.Errorf("read roles dir %q: %w", rolesDir, err)
	}

	deptRoles := make(map[string][]model.MarketRole)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		deptID := resolveDeptID(dirName)

		name, desc := parseIdentityFile(filepath.Join(rolesDir, dirName, "IDENTITY.md"))
		if name == "" {
			name = dirName
		}

		role := model.MarketRole{
			ID:          dirName,
			Name:        name,
			Description: desc,
			Files: model.MarketRoleFiles{
				Identity: filePathPrefix + "/" + dirName + "/IDENTITY.md",
				Soul:     filePathPrefix + "/" + dirName + "/SOUL.md",
				Agents:   filePathPrefix + "/" + dirName + "/AGENTS.md",
			},
		}
		deptRoles[deptID] = append(deptRoles[deptID], role)
	}

	var departments []model.MarketDepartment
	for _, deptID := range deptOrder {
		roles, ok := deptRoles[deptID]
		if !ok {
			continue
		}
		sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
		departments = append(departments, model.MarketDepartment{
			ID:    deptID,
			Name:  deptNameMap[deptID],
			Roles: roles,
		})
		delete(deptRoles, deptID)
	}

	// 未匹配前缀的归入 other
	var otherRoles []model.MarketRole
	for _, roles := range deptRoles {
		otherRoles = append(otherRoles, roles...)
	}
	if len(otherRoles) > 0 {
		sort.Slice(otherRoles, func(i, j int) bool { return otherRoles[i].Name < otherRoles[j].Name })
		departments = append(departments, model.MarketDepartment{
			ID:    "other",
			Name:  deptNameMap["other"],
			Roles: otherRoles,
		})
	}

	return &model.RolesIndex{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Departments: departments,
	}, nil
}

// ── MarketStore：只负责加载 JSON，不做扫描 ──

// roleEntry 内存中的角色条目（含部门信息）
type roleEntry struct {
	role     *model.MarketRole
	deptID   string
	deptName string
}

// MarketStore 工作岗位市场的数据层，独立于主 Store
type MarketStore struct {
	mu    sync.RWMutex
	index *model.RolesIndex    // 完整两层结构（直接对应 JSON）
	byID  map[string]roleEntry // role id → entry（O(1) 查找）
}

// NewMarketStore 从 indexPath 加载角色索引 JSON，找不到或格式错误直接返回错误。
// 需要先运行 gen-roles-index 命令生成该文件。
func NewMarketStore(indexPath string) (*MarketStore, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("roles index not found at %q: run 'go run ./cmd/gen-roles-index' first: %w", indexPath, err)
	}

	var idx model.RolesIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("invalid roles index JSON at %q: %w", indexPath, err)
	}

	ms := &MarketStore{
		index: &idx,
		byID:  make(map[string]roleEntry),
	}
	ms.buildByID()
	return ms, nil
}

// buildByID 从 index 构建 byID 索引（调用方持有锁或初始化时调用）
func (ms *MarketStore) buildByID() {
	ms.byID = make(map[string]roleEntry)
	for i := range ms.index.Departments {
		dept := &ms.index.Departments[i]
		for j := range dept.Roles {
			role := &dept.Roles[j]
			ms.byID[role.ID] = roleEntry{
				role:     role,
				deptID:   dept.ID,
				deptName: dept.Name,
			}
		}
	}
}

// ListDepts 返回所有部门摘要列表
func (ms *MarketStore) ListDepts() []model.MarketDeptSummary {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	result := make([]model.MarketDeptSummary, 0, len(ms.index.Departments))
	for _, dept := range ms.index.Departments {
		result = append(result, model.MarketDeptSummary{
			ID:    dept.ID,
			Name:  dept.Name,
			Count: len(dept.Roles),
		})
	}
	return result
}

// ListRoles 返回过滤后的角色列表（支持部门筛选和关键词搜索）
func (ms *MarketStore) ListRoles(filter model.MarketRoleFilter) []model.MarketRoleListItem {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	q := strings.ToLower(filter.Query)

	var result []model.MarketRoleListItem
	for _, dept := range ms.index.Departments {
		if filter.DeptID != "" && dept.ID != filter.DeptID {
			continue
		}
		for _, role := range dept.Roles {
			if q != "" {
				if !strings.Contains(strings.ToLower(role.Name), q) &&
					!strings.Contains(strings.ToLower(role.Description), q) {
					continue
				}
			}
			result = append(result, model.MarketRoleListItem{
				ID:          role.ID,
				Name:        role.Name,
				Description: role.Description,
				DeptID:      dept.ID,
				DeptName:    dept.Name,
			})
		}
	}
	return result
}

// GetRole 返回角色详情（含三个文件内容）
func (ms *MarketStore) GetRole(id string) (*model.MarketRoleDetail, *transport.AppError) {
	ms.mu.RLock()
	entry, ok := ms.byID[id]
	ms.mu.RUnlock()

	if !ok {
		return nil, transport.NotFound("role not found")
	}

	identity, err := os.ReadFile(entry.role.Files.Identity)
	if err != nil {
		return nil, transport.NotFound("role files not found")
	}
	soul, _ := os.ReadFile(entry.role.Files.Soul)
	agents, _ := os.ReadFile(entry.role.Files.Agents)

	return &model.MarketRoleDetail{
		MarketRoleListItem: model.MarketRoleListItem{
			ID:          entry.role.ID,
			Name:        entry.role.Name,
			Description: entry.role.Description,
			DeptID:      entry.deptID,
			DeptName:    entry.deptName,
		},
		IdentityContent: string(identity),
		SoulContent:     string(soul),
		AgentsContent:   string(agents),
	}, nil
}

// WriteRoleZip 将角色包三个文件打包为 zip 并写入 ResponseWriter（流式，无临时文件）
func (ms *MarketStore) WriteRoleZip(w http.ResponseWriter, id string) *transport.AppError {
	ms.mu.RLock()
	entry, ok := ms.byID[id]
	ms.mu.RUnlock()

	if !ok {
		return transport.NotFound("role not found")
	}

	fileNames := []string{"IDENTITY.md", "SOUL.md", "AGENTS.md"}
	filePaths := []string{entry.role.Files.Identity, entry.role.Files.Soul, entry.role.Files.Agents}

	// 响应头必须在 zip.NewWriter 之前设置
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, id))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer zw.Close()

	for i, name := range fileNames {
		data, err := os.ReadFile(filePaths[i])
		if err != nil {
			continue
		}
		f, err := zw.Create(filepath.Join(id, name))
		if err != nil {
			continue
		}
		_, _ = f.Write(data)
	}
	return nil
}

// resolveDeptID 从目录名解析部门 ID，优先匹配双段前缀
func resolveDeptID(dirName string) string {
	parts := strings.SplitN(dirName, "-", 3)
	if len(parts) >= 2 {
		if id, ok := deptPrefixMap[parts[0]+"-"+parts[1]]; ok {
			return id
		}
	}
	if id, ok := deptPrefixMap[parts[0]]; ok {
		return id
	}
	return "other"
}

// parseIdentityFile 解析 IDENTITY.md，返回角色名称和描述
func parseIdentityFile(path string) (name, desc string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if name == "" {
			name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		desc = line
		break
	}
	return name, desc
}
