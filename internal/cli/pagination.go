package cli

import (
	"context"
	"fmt"

	"github.com/dnscaleou/dnscale-go/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	offset, limit int
	all           bool
}

func (o *listOptions) flags(cmd *cobra.Command, max int) {
	cmd.Flags().IntVar(&o.offset, "offset", 0, "Starting offset (zero-based)")
	cmd.Flags().IntVar(&o.limit, "limit", 50, fmt.Sprintf("Page size (1–%d)", max))
	cmd.Flags().BoolVar(&o.all, "all", false, "Collect all remaining pages before producing output")
}
func (o listOptions) validate(max int) error {
	if o.offset < 0 || o.limit < 1 || o.limit > max {
		return usageError(fmt.Sprintf("offset must be nonnegative and limit must be 1–%d", max))
	}
	return nil
}

// Follow the server's returned count and has_more. Reject incomplete or repeated
// pages rather than silently returning a partial inventory as complete.
func collectPages[T any](ctx context.Context, options listOptions, fetch func(context.Context, int, int) ([]T, api.Pagination, error), id func(T) string) ([]T, *paginationInfo, error) {
	items := make([]T, 0)
	seen := make(map[string]bool)
	position := options.offset
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		page, meta, err := fetch(ctx, position, options.limit)
		if err != nil {
			return nil, nil, err
		}
		count := len(page)
		next := position + count
		if meta.Offset != position || meta.Limit != options.limit || meta.Count != count || count > options.limit || meta.Total < 0 || next < position || (count > 0 && next > meta.Total) || meta.HasMore != (next < meta.Total) || (meta.HasMore && count == 0) {
			return nil, nil, resultError("invalid_pagination", "API returned inconsistent or non-progressing pagination metadata")
		}
		for _, item := range page {
			key := id(item)
			if key == "" || seen[key] {
				return nil, nil, resultError("invalid_pagination", "API returned a missing or repeated ID; reload the listing")
			}
			seen[key] = true
		}
		items = append(items, page...)
		info := &paginationInfo{Offset: options.offset, Limit: options.limit, Returned: len(items), Total: meta.Total, HasMore: meta.HasMore}
		if meta.HasMore {
			info.NextOffset = &next
		}
		if !options.all || !meta.HasMore {
			return items, info, nil
		}
		position = next
	}
}
