package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestPerfMetricIncrementExprQualifiesPostgreSQLColumn(t *testing.T) {
	originalMainDatabaseType := common.MainDatabaseType()
	common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
	t.Cleanup(func() {
		common.SetMainDatabaseType(originalMainDatabaseType)
	})

	expr := perfMetricIncrementExpr("generation_ms", 12)
	if expr.SQL != `"perf_metrics"."generation_ms" + ?` {
		t.Fatalf("SQL = %q, want PostgreSQL-qualified perf_metrics column", expr.SQL)
	}
	if len(expr.Vars) != 1 || expr.Vars[0] != int64(12) {
		t.Fatalf("Vars = %#v, want [12]", expr.Vars)
	}
}

func TestPerfMetricIncrementExprQualifiesGenericColumn(t *testing.T) {
	originalMainDatabaseType := common.MainDatabaseType()
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		common.SetMainDatabaseType(originalMainDatabaseType)
	})

	expr := perfMetricIncrementExpr("generation_ms", 12)
	if expr.SQL != "perf_metrics.generation_ms + ?" {
		t.Fatalf("SQL = %q, want generic qualified perf_metrics column", expr.SQL)
	}
}
