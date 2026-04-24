package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ModelPricing 定义模型定价覆写表。
//
// mkx 2026-04-24：嵌入 JSON 仍是默认数据源，此表只保存管理员覆写价和自定义模型。
type ModelPricing struct {
	ent.Schema
}

// Annotations 返回模型定价表名配置。
func (ModelPricing) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "model_pricings"},
	}
}

// Fields 定义可覆写的四个 per-token 价格字段及元信息。
func (ModelPricing) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_name").
			MaxLen(200).
			NotEmpty().
			Unique(),
		field.Float("input_cost_per_token").
			Optional().
			Nillable(),
		field.Float("output_cost_per_token").
			Optional().
			Nillable(),
		field.Float("cache_read_input_token_cost").
			Optional().
			Nillable(),
		field.Float("cache_creation_input_token_cost").
			Optional().
			Nillable(),
		field.Bool("is_custom").
			Default(false),
		field.Text("note").
			Default(""),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

// Indexes 定义管理列表排序需要的索引。
func (ModelPricing) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("updated_at"),
	}
}
