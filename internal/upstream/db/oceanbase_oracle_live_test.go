//go:build oceanbase_live

package db

import (
	"os"
	"strconv"
	"testing"

	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

func TestOceanBaseOracleLive(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("NAVIFYNE_OB_PORT"))
	if err != nil {
		t.Fatalf("invalid NAVIFYNE_OB_PORT: %v", err)
	}

	ob := &OceanBaseDB{}
	cfg := connection.ConnectionConfig{
		Type:              "oceanbase",
		Host:              os.Getenv("NAVIFYNE_OB_HOST"),
		Port:              port,
		User:              os.Getenv("NAVIFYNE_OB_USER"),
		Password:          os.Getenv("NAVIFYNE_OB_PASSWORD"),
		Database:          os.Getenv("NAVIFYNE_OB_DATABASE"),
		ConnectionParams:  os.Getenv("NAVIFYNE_OB_PARAMS"),
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
