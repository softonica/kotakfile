package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Announcement holds the schema definition for the Announcement entity.
type Announcement struct {
	ent.Schema
}

// Fields of the Announcement.
func (Announcement) Fields() []ent.Field {
	return []ent.Field{
		field.String("title").
			NotEmpty(),
		field.Text("content").
			Optional(),
		field.Bool("is_active").
			Default(true),
		field.Bool("is_popup").
			Default(false),
	}
}

// Edges of the Announcement.
func (Announcement) Edges() []ent.Edge {
	return nil
}

func (Announcement) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
