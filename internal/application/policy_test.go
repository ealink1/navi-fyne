package application

import (
	"github.com/ealink1/navi-fyne/internal/domain"
	"testing"
)

func TestSQLPolicySeesMutationsOutsideLiteralsAndComments(t *testing.T) {
	d, _ := domain.Resolve("postgres")
	cases := []struct {
		text         string
		write, error bool
	}{
		{`SELECT 'DELETE FROM users', "update" FROM users`, false, false},
		{"/* outer /* DELETE */ comment */ SELECT 1 -- DROP\n", false, false},
		{`WITH x AS (DELETE FROM users RETURNING *) SELECT * FROM x`, true, false},
		{`SELECT 1; DROP TABLE users`, true, false},
		{`SELECT $$ DELETE FROM users $$`, false, false},
		{`SELECT $tag$ hi; UPDATE x $tag$`, false, false},
		{`SELECT 1 INTO OUTFILE '/tmp/file'`, true, false},
		{`EXPLAIN ANALYZE UPDATE users SET x=1`, true, false},
		{`/*!50000 DELETE FROM users */ SELECT 1`, false, true},
		{`SELECT 'unfinished`, false, true},
		{`SELECT 1; BEGIN`, false, true},
	}
	for _, test := range cases {
		write, err := Classify(d, domain.Execution{Text: test.text})
		if (err != nil) != test.error || err == nil && write != test.write {
			t.Errorf("%q: write=%v error=%v", test.text, write, err)
		}
	}
}
func TestProtocolPolicyGuardsReadEffectsAndServerAdministration(t *testing.T) {
	cases := []struct {
		kind, text   string
		write, error bool
	}{
		{"redis", "GET key", false, false}, {"redis", "SET key value", true, false}, {"redis", "CONFIG SET dir /tmp", false, true}, {"redis", "EVAL script 0", false, true}, {"redis", "KEYS *", false, true},
		{"rabbitmq", "SELECT * FROM queue LIMIT 10", true, false}, {"kafka", "SELECT * FROM topic LIMIT 10", false, false}, {"mqtt", `{"publish":"test","payload":"hello"}`, true, false},
		{"nacos", `{"op":"publish"}`, true, false}, {"nacos", `{"op":"get"}`, false, false}, {"qdrant", `{"upsert":"c"}`, true, false}, {"qdrant", `{"search":"c","vector":[1]}`, false, false},
		{"elasticsearch", "DELETE /index", true, false}, {"elasticsearch", "GET /index/_search", false, false},
		{"mongodb", `{"find":"c","filter":{}}`, false, false},
		{"mongodb", `{"aggregate":"c","pipeline":[{"$out":"other"}],"cursor":{}}`, true, false},
		{"mongodb", `{"aggregate":"c","pipeline":[{"$match":{}}],"cursor":{}}`, false, false},
		{"mongodb", `{"findAndModify":"c","update":{"$set":{"x":1}}}`, true, false},
		{"mongodb", `{"dropDatabase":1}`, true, false},
		{"mongodb", `{"eval":"code"}`, true, false},
	}
	for _, test := range cases {
		d, _ := domain.Resolve(test.kind)
		write, err := Classify(d, domain.Execution{Text: test.text})
		if (err != nil) != test.error || err == nil && write != test.write {
			t.Errorf("%s %s: write=%v error=%v", test.kind, test.text, write, err)
		}
	}
}
