package filter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

type BaseParams struct {
	Search         string                 `json:"search" form:"search"`
	Filters        map[string]interface{} `json:"filters" form:"filters"`
	OrderBy        string                 `json:"order_by" form:"order_by"`
	OrderDirection string                 `json:"order_direction" form:"order_direction"`
	Page           int                    `json:"page" form:"page"`
	Limit          int                    `json:"limit" form:"limit"`
	Offset         int                    `json:"offset" form:"offset"`
	Columns        []string               `json:"columns" form:"columns"`
}

func GetBaseParams(ctx *gin.Context, defOrderBy, defOrderDirection string, defLimit int) (req BaseParams, err error) {
	err = ctx.Bind(&req)
	if err != nil {
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 10000 {
		req.Limit = defLimit
	}
	if req.OrderBy == "" {
		req.OrderBy = defOrderBy
	}
	validDirs := map[string]bool{"asc": true, "desc": true}
	if !validDirs[strings.ToLower(req.OrderDirection)] {
		req.OrderDirection = defOrderDirection
	}
	req.Offset = (req.Page - 1) * req.Limit

	if req.Filters == nil {
		req.Filters = make(map[string]interface{})
	}
	if filters, ok := ctx.GetQueryMap("filters"); ok {
		for k, v := range filters {
			var jsonVal interface{}
			if err := json.Unmarshal([]byte(v), &jsonVal); err == nil {
				req.Filters[k] = jsonVal
			} else {
				req.Filters[k] = v
			}
		}
	}
	return
}

func whitelistTransform(
	filters map[string]interface{},
	allowed []string,
	transform func(interface{}) interface{},
) map[string]interface{} {
	if filters == nil {
		return nil
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}

	out := make(map[string]interface{}, len(allowedSet))
	for k, v := range filters {
		if _, ok := allowedSet[k]; ok {
			if transform != nil {
				out[k] = transform(v)
			} else {
				out[k] = v
			}
		}
	}
	return out
}

func WhitelistFilter(filters map[string]interface{}, allowed []string) map[string]interface{} {
	return whitelistTransform(filters, allowed, nil)
}

func WhitelistStringFilter(filters map[string]interface{}, allowed []string) map[string]interface{} {
	return whitelistTransform(filters, allowed, func(v interface{}) interface{} {
		switch t := v.(type) {
		case nil:
			return ""
		case string:
			return t
		case fmt.Stringer:
			return t.String()
		case []string:
			return strings.Join(t, ",")
		default:
			return fmt.Sprintf("%v", v)
		}
	})
}
