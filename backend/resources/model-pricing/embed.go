// mkx: 将模型定价 JSON 嵌入二进制，取代运行时远程同步 (2026-04-24)
// 本包仅作为定价数据的唯一权威来源被 service 层引用。
package modelpricing

import _ "embed"

//go:embed model_prices_and_context_window.json
var JSON []byte
