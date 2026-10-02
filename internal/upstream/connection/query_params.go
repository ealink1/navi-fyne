package connection

// QueryParamBinding 是查询编辑器运行时参数的一次输入：参数名、声明类型与值。
// 值以 JSON 反序列化形态传递，由后端统一转换并经驱动绑定执行；
// 值不进入查询历史、审计与日志，历史与审计只记录含 :name 的 SQL 原文。
type QueryParamBinding struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Value any    `json:"value,omitempty"`
}
