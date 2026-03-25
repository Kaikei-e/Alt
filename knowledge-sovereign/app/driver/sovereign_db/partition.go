package sovereign_db

import (
	"fmt"
	"time"
)

// PartitionSpec describes a partition to be created.
type PartitionSpec struct {
	Name string
	DDL  string
}

// GeneratePartitionDDL generates DDL statements for monthly partitions of the given table.
// It creates `count` partitions starting from `startMonth`.
func GeneratePartitionDDL(tableName string, startMonth time.Time, count int) []PartitionSpec {
	// Normalize to first of month in UTC
	current := time.Date(startMonth.Year(), startMonth.Month(), 1, 0, 0, 0, 0, time.UTC)

	specs := make([]PartitionSpec, 0, count)
	for i := 0; i < count; i++ {
		next := current.AddDate(0, 1, 0)
		name := fmt.Sprintf("%s_y%04dm%02d", tableName, current.Year(), current.Month())
		ddl := fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
			name,
			tableName,
			current.Format("2006-01-02"),
			next.Format("2006-01-02"),
		)
		specs = append(specs, PartitionSpec{Name: name, DDL: ddl})
		current = next
	}
	return specs
}
