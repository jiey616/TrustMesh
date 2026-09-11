// Package ssoverify 演示外部平台如何校验 TrustMesh 签发的 HS256 JWT。
//
// 与后端 backend/internal/auth/external_token.go 的 ExternalClaims 字段、算法、
// 签名密钥完全一致。把本文件编译进你的外部平台校验服务即可。
package main

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-jwt/jwt/v5"
)

// ExternalTokenIssuer 必须与后端 ExternalTokenIssuer 一致。
const ExternalTokenIssuer = "trustmesh"

// ExternalClaims 必须与后端 ExternalClaims 字段一致（json tag 对齐）。
type ExternalClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	Scope     string `json:"scope,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	jwt.RegisteredClaims
}

// Verify 校验 TrustMesh SSO token。
//
//	clientSecret : 创建外部应用时下发的 client_secret（平台侧保管）
//	clientID     : 本平台自己的 client_id，必须与 token 的 aud 匹配
//
// 返回解析后的 claims；任何校验失败都返回 error。
func Verify(tokenStr, clientSecret, clientID string) (*ExternalClaims, error) {
	if clientSecret == "" {
		return nil, errors.New("client_secret not configured")
	}
	if clientID == "" {
		return nil, errors.New("client_id not configured")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &ExternalClaims{}, func(t *jwt.Token) (any, error) {
		// 1) 算法强制 HS256，拒绝 none / RS*/ES* 等算法混淆攻击。
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(clientSecret), nil
	},
		// 2) 校验 iss。
		jwt.WithIssuer(ExternalTokenIssuer),
		// 3) 校验 aud 包含本平台 client_id。
		jwt.WithAudience(clientID),
		// 4) 校验 exp（库默认会校验，这里显式声明语义）。
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("token invalid: %w", err)
	}

	claims, ok := token.Claims.(*ExternalClaims)
	if !ok || !token.Valid {
		return nil, errors.New("token claims invalid")
	}
	return claims, nil
}

// ExtractTokenFromURL 从外部平台收到的跳转 URL 中取 ?token= 查询参数。
func ExtractTokenFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	t := u.Query().Get("token")
	if t == "" {
		return "", errors.New("missing ?token= in launch url")
	}
	return t, nil
}

// Example 演示完整流程：从 launch_url 取 token -> 校验 -> 账号映射。
func main() {
	const (
		clientSecret = "YOUR_CLIENT_SECRET_HERE" // 从配置读取，切勿硬编码进仓库
		clientID     = "your-platform-client-id"
		launchURL    = "https://your-platform.example.com/home?token=PASTE_TOKEN_HERE"
	)

	tokenStr, err := ExtractTokenFromURL(launchURL)
	if err != nil {
		panic(err)
	}

	claims, err := Verify(tokenStr, clientSecret, clientID)
	if err != nil {
		panic(err)
	}

	// 5) 防重放（可选但推荐）：把 claims.ID (jti) 记作一次性消费，重复提交拒绝。
	//    例如用 Redis SETNX jti 1 EX 300。

	// 6) 账号映射：优先用 email，其次 user_id，必要时按需创建本地账号。
	subject := claims.Subject // == claims.UserID
	fmt.Printf("SSO login user_id=%s email=%s name=%s scope=%s\n",
		subject, claims.Email, claims.Name, claims.Scope)

	// 建立本地会话 / 设置 cookie 等……
}
