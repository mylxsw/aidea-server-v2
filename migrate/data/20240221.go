package data

import "github.com/mylxsw/eloquent/migrate"

func Migrate20240221(m *migrate.Manager) {

	m.Schema("20240221").Create("cache", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.String("key", 255).Nullable(false).Unique().Comment("Key")
		builder.Text("value").Nullable(true).Comment("Value")
		builder.Timestamp("valid_until", 0).Nullable(true).Index("idx_valid_until").Comment("expire period")
		builder.Timestamps(0)
	})

	m.Schema("20240221").Create("debt", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.Integer("user_id", false, true).Nullable(false).Comment("User ID")
		builder.Integer("used", false, true).Nullable(false).Comment("Used")
		builder.Timestamps(0)
	})

	m.Schema("20240221").Create("events", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.String("event_type", 50).Nullable(false).Comment("Event Type")
		builder.Text("payload").Nullable(true).Comment("Payload")
		builder.String("status", 20).Nullable(false).Comment("Status")
		builder.Timestamps(0)

		builder.Index("idx_status_type", "status", "event_type")
	})

	m.Schema("20240221").Create("queue_tasks", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.Timestamps(0)

		builder.String("title", 255).Nullable(false).Comment("Title")
		builder.String("task_id", 255).Nullable(false).Comment("Task ID")
		builder.String("task_type", 50).Nullable(false).Comment("Task Type")
		builder.String("queue_name", 255).Nullable(false).Comment("Queue Name")
		builder.Text("payload").Nullable(true).Comment("Payload")
		builder.Text("result").Nullable(true).Comment("Result")
		builder.String("status", 20).Nullable(false).Comment("Status")

		builder.Index("idx_task_id", "task_id")
	})

	m.Schema("20240221").Create("quota", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.Timestamps(0)

		builder.Integer("user_id", false, true).Nullable(false).Comment("User ID")
		builder.Integer("quota", false, true).Nullable(false).Comment("Quota")
		builder.Integer("rest", false, true).Nullable(false).Comment("Rest")
		builder.Timestamp("period_end_at", 0).Nullable(true).Comment("expire period")
		builder.String("note", 255).Nullable(true).Comment("Note")
		builder.String("payment_id", 64).Nullable(true).Comment("Payment ID")

		builder.Index("idx_user_period", "user_id", "period_end_at")
	})

	m.Schema("20240221").Create("quota_statistics", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.Timestamps(0)

		builder.Integer("user_id", false, true).Nullable(false).Comment("User ID")
		builder.Integer("used", false, true).Nullable(false).Comment("当日使用额度")
		builder.Date("cal_date").Nullable(true).Comment("统计日期")

		builder.Index("idx_user_id", "user_id")
	})

	m.Schema("20240221").Create("quota_usage", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.Timestamps(0)

		builder.Integer("user_id", false, true).Nullable(false).Comment("User ID")
		builder.Integer("used", false, true).Nullable(false).Comment("Used")
		builder.Json("quota_ids").Nullable(true).Comment("使用的额度 ID 列表")
		builder.Integer("debt", false, true).Nullable(false).Comment("额度不够扣除的债务")
		builder.Json("meta").Nullable(true).Comment("额外信息")

		builder.Index("idx_user_created_at", "user_id", "created_at")
	})

	m.Schema("20240221").Create("user_custom", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.Timestamps(0)

		builder.Integer("user_id", false, true).Nullable(false).Unique().Comment("User ID")
		builder.Json("config").Nullable(true).Comment("Config")
	})

	m.Schema("20240221").Create("users", func(builder *migrate.Builder) {
		builder.Increments("id")
		builder.Timestamps(0)

		builder.TinyInteger("user_type", false, true).Nullable(false).Default(migrate.RawExpr("0")).Comment("User Type: 0-普通用户，1-内部用户，2-测试用户，3-例外用户")
		builder.String("phone", 20).Nullable(true).Unique().Comment("Phone")
		builder.String("email", 100).Nullable(true).Unique().Comment("Email")
		builder.String("password", 255).Nullable(true).Comment("Password")
		builder.String("realname", 255).Nullable(true).Comment("Name")
		builder.String("avatar", 255).Nullable(true).Comment("Avatar URL")
		builder.String("status", 20).Nullable(false).Comment("Status")
		builder.String("apple_uid", 255).Nullable(true).Unique().Comment("Apple UID")
		builder.String("union_id", 255).Nullable(true).Unique().Comment("Wechat Union ID")
		builder.String("prefer_signin_method", 10).Nullable(true).Comment("Prefer Signin Method: passsword, verify_code")
		builder.Integer("invited_by", false, true).Nullable(true).Comment("Invited By")
		builder.String("invite_code", 20).Nullable(true).Unique().Comment("Invite Code")
	})
}
