package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestPerfMetricIncrementExprQualifiesPostgreSQLColumn(t *testing.T) {
	originalPostgreSQL := common.UsingPostgreSQL
	common.UsingPostgreSQL = true
	t.Cleanup(func() {
		common.UsingPostgreSQL = originalPostgreSQL
	})

	expr := perfMetricIncrementExpr("generation_ms", 12)
	if expr.SQL != `"perf_metrics"."generation_ms" + ?` {
		t.Fatalf("SQL = %q, want PostgreSQL-qualified perf_metrics column", expr.SQL)
	}
	if len(expr.Vars) != 1 || expr.Vars[0] != int64(12) {
		t.Fatalf("Vars = %#v, want [12]", expr.Vars)
	}
}

func TestPerfMetricIncrementExprKeepsGenericColumn(t *testing.T) {
	originalPostgreSQL := common.UsingPostgreSQL
	common.UsingPostgreSQL = false
	t.Cleanup(func() {
		common.UsingPostgreSQL = originalPostgreSQL
	})

	expr := perfMetricIncrementExpr("generation_ms", 12)
	if expr.SQL != "generation_ms + ?" {
		t.Fatalf("SQL = %q, want generic unqualified column", expr.SQL)
	}
}
