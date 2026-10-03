//go:build oceanbase_live

package db

import (
	"os"
	"strconv"
	"testing"

	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestOceanBaseOracleLive(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("SUPERLINK_OB_PORT"))
	if err != nil {
		t.Fatalf("invalid SUPERLINK_OB_PORT: %v", err)
	}

	ob := &OceanBaseDB{}
	cfg := connection.ConnectionConfig{
		Type:              "oceanbase",
		Host:              os.Getenv("SUPERLINK_OB_HOST"),
		Port:              port,
		User:              os.Getenv("SUPERLINK_OB_USER"),
		Password:          os.Getenv("SUPERLINK_OB_PASSWORD"),
		Database:          os.Getenv("SUPERLINK_OB_DATABASE"),
		ConnectionParams:  os.Getenv("SUPERLINK_OB_PARAMS"),
		OceanBaseProtocol: "oracle",
		Timeout:           10,
	}
	if err := ob.Connect(cfg); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer ob.Close()

	rows, fields, err := ob.Query("select 1 from dual")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(fields) != 1 || len(rows) != 1 {
		t.Fatalf("unexpected result fields=%v rows=%v", fields, rows)
	}
}
