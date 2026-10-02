package db

import "testing"

func TestBuildDuckDBAttachStatement(t *testing.T) {
	cases := []struct {
		name string
		spec ExternalAttachSpec
		want string
	}{
		{
			// mysql 扩展把 ATTACH 的 path 当主机 DSN：库名必须走 SECRET 的 DATABASE
			name: "mysql read only uses empty path and secret database",
			spec: ExternalAttachSpec{Kind: ExternalAttachKindMySQL, Database: "orders", Alias: "a", SecretName: "s1", ReadOnly: true},
			want: `ATTACH '' AS "a" (TYPE MYSQL, SECRET s1, READ_ONLY)`,
		},
		{
			name: "mysql read write",
			spec: ExternalAttachSpec{Kind: ExternalAttachKindMySQL, Database: "orders", Alias: "a", SecretName: "s1"},
			want: `ATTACH '' AS "a" (TYPE MYSQL, SECRET s1)`,
		},
		{
			name: "postgres uses secret database and empty path",
			spec: ExternalAttachSpec{Kind: ExternalAttachKindPostgres, Database: "warehouse", Alias: "pg", SecretName: "s2", ReadOnly: true},
			want: `ATTACH '' AS "pg" (TYPE POSTGRES, SECRET s2, READ_ONLY)`,
		},
		{
			name: "sqlite file",
			spec: ExternalAttachSpec{Kind: ExternalAttachKindSQLite, FilePath: "D:/data/x.db", Alias: "lite"},
			want: `ATTACH 'D:/data/x.db' AS "lite" (TYPE SQLITE)`,
		},
		{
			name: "reserved word alias is quoted",
			spec: ExternalAttachSpec{Kind: ExternalAttachKindSQLite, FilePath: "x.db", Alias: "order"},
			want: `ATTACH 'x.db' AS "order" (TYPE SQLITE)`,
		},
		{
			name: "duckdb native read only",
			spec: ExternalAttachSpec{Kind: ExternalAttachKindDuckDB, FilePath: "a.duckdb", Alias: "ext", ReadOnly: true},
			want: `ATTACH 'a.duckdb' AS "ext" (READ_ONLY)`,
		},
		{
			name: "duckdb native read write",
			spec: ExternalAttachSpec{Kind: ExternalAttachKindDuckDB, FilePath: "a.duckdb", Alias: "ext"},
			want: `ATTACH 'a.duckdb' AS "ext"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildDuckDBAttachStatement(tc.spec); got != tc.want {
				t.Fatalf("statement = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSameExternalAttachmentIdentityIgnoresPassword(t *testing.T) {
	base := duckDBAttachmentSpec{
		kind: ExternalAttachKindMySQL, host: "h", port: 3306,
		user: "u", password: "old", database: "d", readOnly: true,
		connectionID: "conn-1",
	}
	rotated := base
	rotated.password = "new"
	if !sameExternalAttachmentIdentity(base, rotated) {
		t.Fatal("password rotation must still count as same source")
	}
	otherHost := base
	otherHost.password = "new"
	otherHost.host = "other"
	if sameExternalAttachmentIdentity(base, otherHost) {
		t.Fatal("different host must be a conflict")
	}
	// 只读/读写切换属同源重跑：走替换重建，而非别名冲突（上游审查 P1-6）
	modeSwitched := base
	modeSwitched.password = "new"
	modeSwitched.readOnly = false
	if !sameExternalAttachmentIdentity(base, modeSwitched) {
		t.Fatal("readonly mode switch must still count as same source")
	}
	otherConnection := base
	otherConnection.password = "new"
	otherConnection.connectionID = "conn-2"
	if sameExternalAttachmentIdentity(base, otherConnection) {
		t.Fatal("different connection must be a conflict")
	}
}
