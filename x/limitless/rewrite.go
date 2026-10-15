package limitless

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/superdurable/xtemporal/common/log/tag"
)

var errMissingGetContext = errors.New("postgres12_pgx DB does not implement GetContext")

func (d *db) RewriteSchemaStatements(statements []string) []string {
	getter, ok := d.DB.(interface {
		GetContext(context.Context, any, string, ...any) error
	})
	if !ok {
		d.logger.Error("unable to detect Limitless server", tag.Error(errMissingGetContext))
		return statements
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var isLimitless bool
	err := getter.GetContext(ctx, &isLimitless, "SELECT to_regproc('rds_aurora.limitless_alter_table_type_sharded') IS NOT NULL")
	if err != nil {
		d.logger.Error("unable to detect Limitless server", tag.Error(err))
		return statements
	}
	if isLimitless {
		return statements
	}
	rewritten := make([]string, 0, len(statements))
	for _, stmt := range statements {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(stmt)), "CALL RDS_AURORA.LIMITLESS_") {
			continue
		}
		rewritten = append(rewritten, stmt)
	}
	return rewritten
}
