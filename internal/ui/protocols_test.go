package ui

import "testing"

func TestPreviewSQLQuotesQualifiedIdentifiersAndCompatibleLimits(t *testing.T) {
	for _, test := range []struct{ kind, name, expected string }{
		{"oracle", `"Owner"."a.b"`, `SELECT * FROM "Owner"."a.b" WHERE ROWNUM <= 100;`},
		{"trino", `catalog.schema."a.b"`, `SELECT * FROM "catalog"."schema"."a.b" LIMIT 100;`},
		{"sqlserver", `[db].[dbo].[a]]b]`, `SELECT TOP 100 * FROM [db].[dbo].[a]]b];`},
		{"iris", `Sample.Person`, `SELECT TOP 100 * FROM "Sample"."Person";`},
		{"sqlite", `a.b`, `SELECT * FROM "a.b" LIMIT 100;`},
		{"mysql", "`a.b`.`c``d`", "SELECT * FROM `a.b`.`c``d` LIMIT 100;"},
		{"iotdb", `root.group.device`, "SELECT * FROM root.`group`.`device` LIMIT 100;"},
		{"postgres", `x"; DROP TABLE users; --`, `SELECT * FROM "x""; DROP TABLE users; --" LIMIT 100;`},
	} {
		t.Run(test.kind+test.name, func(t *testing.T) {
			if got := previewSQL(test.kind, test.name); got != test.expected {
				t.Fatalf("preview query: %q, expected %q", got, test.expected)
			}
		})
	}
}
