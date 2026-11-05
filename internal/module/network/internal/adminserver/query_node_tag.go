package adminserver

import (
	"context"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
)

// QueryNodeTag lists every tag any node carries, once.
func (s *Service) QueryNodeTag(ctx context.Context) (*dto.QueryNodeTagResponse, error) {
	nodeTags, err := s.deps.Store.Node().QueryNodeTags(ctx)
	if err != nil {
		logger.WithContext(ctx).Errorw("[QueryNodeTag] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, xerr.Errorf(xerr.DatabaseQueryError, "[QueryNodeTag] Query Database Error")
	}
	var tags []string
	for _, item := range nodeTags {
		tags = append(tags, strings.Split(item, ",")...)
	}

	return &dto.QueryNodeTagResponse{
		Tags: slicesx.RemoveDuplicateElements(tags...),
	}, nil
}
