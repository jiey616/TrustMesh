#!/usr/bin/env node
/**
 * 把预置的自签证书与 network_security_config 套用到 Capacitor 生成的安卓工程。
 *
 * 用法（在 frontend-v2 目录下）：
 *   npx cap add android          # 首次生成原生工程
 *   node android-config/apply.mjs
 *
 * 幂等：重复执行不会重复插入属性，也不会破坏已有文件。
 * 退出码：0 成功；1 前置条件不满足或写入失败。
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const FV2 = path.resolve(HERE, '..');
const ANDROID_MAIN = path.join(FV2, 'android', 'app', 'src', 'main');
const MANIFEST = path.join(ANDROID_MAIN, 'AndroidManifest.xml');

const SRC_CERT = path.join(HERE, 'prod_ca.crt');
const SRC_XML = path.join(HERE, 'network_security_config.xml');
const DST_CERT = path.join(ANDROID_MAIN, 'res', 'raw', 'prod_ca.crt');
const DST_XML = path.join(ANDROID_MAIN, 'res', 'xml', 'network_security_config.xml');

const log = [];
const fail = (msg) => {
  log.push('FAIL: ' + msg);
  fs.writeFileSync(path.join(HERE, 'apply_last_run.txt'), log.join('\n') + '\n', 'utf8');
  process.exit(1);
};

if (!fs.existsSync(MANIFEST)) {
  fail('未找到 ' + MANIFEST + '，请先在 frontend-v2 下执行 `npx cap add android`');
}
if (!fs.existsSync(SRC_CERT)) fail('缺少源证书 ' + SRC_CERT);
if (!fs.existsSync(SRC_XML)) fail('缺少源配置 ' + SRC_XML);

fs.mkdirSync(path.dirname(DST_CERT), { recursive: true });
fs.mkdirSync(path.dirname(DST_XML), { recursive: true });
fs.copyFileSync(SRC_CERT, DST_CERT);
log.push('copied cert -> ' + path.relative(FV2, DST_CERT) + ' (' + fs.statSync(DST_CERT).size + ' B)');

// --negative：故意写一份**去掉 domain-config** 的配置，用于反证。
// 它仍然声明 networkSecurityConfig 属性（保证「配置存在但没放行自签证书」这一条件独立），
// 预期结果是 fetch 抛 "Trust anchor for certification path not found"。
const NEGATIVE = process.argv.includes('--negative');
if (NEGATIVE) {
  const stripped = `<?xml version="1.0" encoding="utf-8"?>
<!-- 反证用：刻意不含 domain config，自签证书不被信任。由 apply.mjs 的 negative 模式生成，勿提交。
     注意：XML 注释里禁止出现连续两个减号（AAPT2 会报 "注释中不允许出现字符串"），
     所以这里不能写成带两个减号的命令行开关形式。 -->
<network-security-config>
    <base-config cleartextTrafficPermitted="false">
        <trust-anchors>
            <certificates src="system" />
        </trust-anchors>
    </base-config>
</network-security-config>
`;
  fs.writeFileSync(DST_XML, stripped, 'utf8');
  log.push('NEGATIVE MODE: wrote stripped xml (no domain-config) -> ' + path.relative(FV2, DST_XML));
} else {
  fs.copyFileSync(SRC_XML, DST_XML);
  log.push('copied xml  -> ' + path.relative(FV2, DST_XML));
}

const ATTR = 'android:networkSecurityConfig="@xml/network_security_config"';
let manifest = fs.readFileSync(MANIFEST, 'utf8');

if (manifest.includes('android:networkSecurityConfig=')) {
  log.push('manifest already declares networkSecurityConfig -> skip patch');
} else {
  const before = manifest;
  manifest = manifest.replace(/<application(\s)/, '<application\n        ' + ATTR + '$1');
  if (manifest === before) fail('manifest 中未找到 <application 标签，无法注入属性（Capacitor 模板可能已变更）');
  fs.writeFileSync(MANIFEST, manifest, 'utf8');
  log.push('patched  -> ' + path.relative(FV2, MANIFEST) + '  (+' + ATTR + ')');
}

if (!manifest.includes('android:usesCleartextTraffic=')) {
  log.push('NOTE: manifest 未显式声明 usesCleartextTraffic；已由 network_security_config 的 base-config 禁止明文');
}

log.push('OK');
fs.writeFileSync(path.join(HERE, 'apply_last_run.txt'), log.join('\n') + '\n', 'utf8');
console.log(log.join('\n'));
