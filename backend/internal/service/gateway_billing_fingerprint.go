package service

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/tidwall/gjson"
)

// billingFingerprintSalt 与官方 CC CLI 的 utils/fingerprint.ts 中使用的盐值一致。
// cc_version 的 3-hex 后缀 = SHA256(salt + msg[4] + msg[7] + msg[20] + version)[:3]。
// 官方算法对下标越界的字符使用 JS 的 undefined 拼接行为（字符串 "undefined"）。
const billingFingerprintSalt = "59cf53e54c78"

// undefinedMarker mirrors JavaScript's String(undefined) when indexing out of range.
const undefinedMarker = "undefined"

// ComputeCCVersionFingerprint 计算 cc_version 的 3-hex 指纹后缀。
// 依赖：首条 role == user 的 message 的第一个 text 内容，取 index 4/7/20 字符。
// 若 messages 为空 / 无 user 消息 / 不是文本块，对应下标当 "undefined" 处理。
func ComputeCCVersionFingerprint(body []byte, version string) string {
	first := extractFirstUserMessageText(body)
	input := billingFingerprintSalt +
		charAtOrUndefined(first, 4) +
		charAtOrUndefined(first, 7) +
		charAtOrUndefined(first, 20) +
		version
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])[:3]
}

// charAtOrUndefined 模拟 JS 的 string[index]：越界返回 "undefined"，
// 否则返回该下标上的单个 UTF-8 字符（rune）对应的字符串。
// JS 的 charAt/index 基于 UTF-16 code unit；对于 BMP 内的 ASCII 文本（最常见的
// 首条 system-reminder/用户输入场景），按 rune 切片结果一致。
func charAtOrUndefined(s string, i int) string {
	if s == "" {
		return undefinedMarker
	}
	runes := []rune(s)
	if i < 0 || i >= len(runes) {
		return undefinedMarker
	}
	return string(runes[i])
}

// extractFirstUserMessageText 返回 messages[] 中首条 role == "user" 消息的
// 第一个可用文本。content 可能是 string、string[]、或 {type,text}[]。
func extractFirstUserMessageText(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return ""
	}
	var out string
	messages.ForEach(func(_, msg gjson.Result) bool {
		if msg.Get("role").String() != "user" {
			return true
		}
		content := msg.Get("content")
		if !content.Exists() {
			return true
		}
		switch {
		case content.Type == gjson.String:
			out = content.String()
		case content.IsArray():
			content.ForEach(func(_, block gjson.Result) bool {
				switch {
				case block.Type == gjson.String:
					out = block.String()
				case block.Get("type").String() == "text":
					if t := block.Get("text"); t.Exists() && t.Type == gjson.String {
						out = t.String()
					}
				}
				return out == ""
			})
		}
		return out == ""
	})
	return out
}
