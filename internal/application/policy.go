package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/esconsole"
	"github.com/ealink1/super-link/internal/upstream/sqlparam"
	"github.com/google/shlex"
)

// Classify is conservative. Database privileges remain the final boundary:
// a SELECT can invoke a server-side function with effects that a lexer cannot know.
func Classify(d domain.Descriptor, e domain.Execution) (bool, error) {
	if strings.TrimSpace(e.Text) == "" {
		return false, errors.New("enter a command first")
	}
	if len(e.Text) > 1<<20 {
		return false, errors.New("command exceeds 1 MiB")
	}
	switch d.Family {
	case domain.Search:
		batch, err := esconsole.ParseSource(e.Text, e.Scope)
		if err != nil {
			return false, err
		}
		if batch.Blocked {
			return false, errors.New("Elasticsearch request is blocked")
		}
		return batch.ContainsWrite, nil
	case domain.Cache:
		args, err := shlex.Split(e.Text)
		if err != nil || len(args) == 0 {
			return false, errors.New("invalid Redis command")
		}
		cmd := strings.ToUpper(args[0])
		if redisBlocked[cmd] {
			return false, fmt.Errorf("%s is disabled in this interactive console", cmd)
		}
		return !redisReads[cmd], nil
	case domain.Configuration:
		var command struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal([]byte(e.Text), &command); err != nil {
			return false, err
		}
		return command.Op == "publish" || command.Op == "delete", nil
	case domain.Message:
		text := strings.ToUpper(strings.TrimSpace(e.Text))
		// Message publication goes only through the driver's Exec JSON contract.
		if strings.HasPrefix(text, "{") {
			return true, nil
		}
		// RabbitMQ peek requeues messages but can affect redelivery metadata/order.
		if d.Key == "rabbitmq" && (strings.HasPrefix(text, "SELECT") || strings.HasPrefix(text, "CONSUME")) {
			return true, nil
		}
		return false, nil
	case domain.Document:
		if strings.HasPrefix(strings.TrimSpace(e.Text), "{") {
			return classifyDocument(e.Text)
		}
	case domain.Vector:
		// Driver QueryContext only exposes read commands; native writes use Exec.
		if strings.HasPrefix(strings.TrimSpace(e.Text), "{") {
			var command map[string]json.RawMessage
			if err := json.Unmarshal([]byte(e.Text), &command); err != nil {
				return false, err
			}
			for key := range command {
				if nativeWrites[strings.ToLower(key)] {
					return true, nil
				}
			}
			return false, nil
		}
	}
	return classifySQL(e.Text, d.Key)
}

var redisReads = wordSet("PING GET MGET TYPE TTL PTTL EXISTS STRLEN GETRANGE SCAN HGET HGETALL HMGET HKEYS HVALS HLEN HSCAN LLEN LRANGE LINDEX SCARD SISMEMBER SMEMBERS SSCAN ZCARD ZRANGE ZREVRANGE ZRANGEBYSCORE ZSCORE ZSCAN XLEN XRANGE XREVRANGE INFO DBSIZE TIME ECHO")
var redisBlocked = wordSet("KEYS MONITOR SUBSCRIBE PSUBSCRIBE SSUBSCRIBE BLPOP BRPOP BZPOPMIN BZPOPMAX XREAD XREADGROUP MULTI EXEC WATCH UNWATCH DISCARD SELECT AUTH HELLO QUIT SHUTDOWN DEBUG MODULE CONFIG ACL CLUSTER SENTINEL MIGRATE REPLICAOF SLAVEOF SYNC PSYNC EVAL EVALSHA FCALL FCALL_RO SCRIPT FUNCTION")
var nativeWrites = wordSetLower("insert insertone insertmany update updateone updatemany delete deleteone deletemany remove replace replaceone drop drop_collection dropcollection delete_collection deletecollection create_collection createcollection upsert add publish create_index createindex delete_index deleteindex")

func classifyDocument(text string) (bool, error) {
	var command map[string]any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := decoder.Decode(&command); err != nil {
		return false, err
	}
	reads := wordSet("find count aggregate distinct listCollections listIndexes collStats dbStats serverStatus ping buildInfo hello isMaster explain")
	read := false
	for key, value := range command {
		if reads[key] {
			read = true
		}
		if nativeWrites[strings.ToLower(key)] {
			return true, nil
		}
		if key == "pipeline" && containsMongoWrite(value) {
			return true, nil
		}
		if key == "explain" && containsMongoWrite(value) {
			return true, nil
		}
	}
	// Unknown MongoDB commands can administer the server or change documents.
	return !read, nil
}
func containsMongoWrite(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, nested := range v {
			if key == "$out" || key == "$merge" || nativeWrites[strings.ToLower(key)] || containsMongoWrite(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range v {
			if containsMongoWrite(nested) {
				return true
			}
		}
	}
	return false
}

func wordSet(words string) map[string]bool {
	result := map[string]bool{}
	for _, word := range strings.Fields(words) {
		result[word] = true
	}
	return result
}
func wordSetLower(words string) map[string]bool { return wordSet(words) }

var sqlMutations = wordSet("INSERT UPDATE DELETE MERGE UPSERT REPLACE CREATE ALTER DROP TRUNCATE GRANT REVOKE COMMENT RENAME VACUUM REINDEX ANALYZE COPY LOAD CALL EXEC EXECUTE DO INTO OUTFILE DUMPFILE LOCK UNLOCK SET ATTACH DETACH PRAGMA OPTIMIZE REFRESH KILL")
var transactionControl = wordSet("BEGIN COMMIT ROLLBACK SAVEPOINT RELEASE START")
var riskyFunctions = wordSet("LOAD_EXTENSION LO_IMPORT LO_EXPORT DBLINK_EXEC NEXTVAL SETVAL PG_TERMINATE_BACKEND PG_CANCEL_BACKEND GET_LOCK RELEASE_LOCK SLEEP PG_SLEEP")

func classifySQL(text, dialect string) (bool, error) {
	tokens, err := sqlTokens(text, dialect)
	if err != nil {
		return false, err
	}
	if len(tokens) == 0 {
		return false, errors.New("empty SQL statement")
	}
	first := true
	write := false
	for _, token := range tokens {
		if token == ";" {
			first = true
			continue
		}
		if transactionControl[token] {
			return false, errors.New("manual transaction control is not enabled in this version")
		}
		if riskyFunctions[token] || sqlMutations[token] {
			write = true
		}
		if first {
			if token != "SELECT" && token != "WITH" && token != "SHOW" && token != "DESCRIBE" && token != "DESC" && token != "EXPLAIN" && token != "VALUES" {
				write = true
			}
			first = false
		}
	}
	return write, nil
}

// sqlTokens ignores literals and comments, supports nested comments, quoted
// identifiers and PostgreSQL dollar strings, and fails closed on incomplete input.
func sqlTokens(text, dialect string) ([]string, error) {
	opts := sqlparam.OptionsForDBType(dialect)
	tokens := []string{}
	r := []rune(text)
	for i := 0; i < len(r); {
		c := r[i]
		if unicode.IsSpace(c) {
			i++
			continue
		}
		if c == '-' && i+1 < len(r) && r[i+1] == '-' && (!opts.DashCommentNeedsSpace || i+2 == len(r) || unicode.IsSpace(r[i+2])) {
			for i < len(r) && r[i] != '\n' {
				i++
			}
			continue
		}
		if c == '#' && opts.HashComments {
			for i < len(r) && r[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(r) && r[i+1] == '*' {
			if i+2 < len(r) && r[i+2] == '!' {
				return nil, errors.New("executable SQL comments are not allowed")
			}
			depth := 1
			i += 2
			for i < len(r) && depth > 0 {
				if i+1 < len(r) && r[i] == '/' && r[i+1] == '*' {
					depth++
					i += 2
				} else if i+1 < len(r) && r[i] == '*' && r[i+1] == '/' {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth != 0 {
				return nil, errors.New("unclosed SQL comment")
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' && opts.BracketIdentifiers {
			backslash := opts.BackslashEscapes || opts.DollarQuotes && c == '\'' && i > 0 && (r[i-1] == 'E' || r[i-1] == 'e')
			end := c
			if c == '[' {
				end = ']'
			}
			i++
			closed := false
			for i < len(r) {
				if r[i] == '\\' && backslash {
					i += 2
					continue
				}
				if r[i] == end {
					if i+1 < len(r) && r[i+1] == end {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, errors.New("unclosed SQL quote")
			}
			continue
		}
		if c == '$' && opts.DollarQuotes {
			j := i + 1
			for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_') {
				j++
			}
			if j < len(r) && r[j] == '$' {
				tag := string(r[i : j+1])
				tail := string(r[j+1:])
				end := strings.Index(tail, tag)
				if end < 0 {
					return nil, errors.New("unclosed dollar string")
				}
				i = j + 1 + len([]rune(tail[:end])) + len([]rune(tag))
				continue
			}
		}
		if c == ';' {
			tokens = append(tokens, ";")
			i++
			continue
		}
		if unicode.IsLetter(c) || c == '_' {
			start := i
			i++
			for i < len(r) && (unicode.IsLetter(r[i]) || unicode.IsDigit(r[i]) || r[i] == '_') {
				i++
			}
			tokens = append(tokens, strings.ToUpper(string(r[start:i])))
			continue
		}
		i++
	}
	return tokens, nil
}
