package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/diwise/context-broker/pkg/ngsild/types/entities"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func QueryEntities[T any](ctx context.Context, broker, tenant, entityType string, attributes []string, callback func(t T)) (count int, err error) {

	logger := logging.GetFromContext(ctx)

	httpClient := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	limit := 50
	offset := 0

	result := make([]T, 0, limit)

	entityAttributes := ""

	if len(attributes) > 0 {
		entityAttributes = "&attrs=" + strings.Join(attributes, ",")
	}

	for {
		var req *http.Request
		var resp *http.Response
		var respBody []byte

		url := fmt.Sprintf(
			"%s/ngsi-ld/v1/entities?type=%s%s&limit=%d&offset=%d&options=keyValues",
			broker, entityType, entityAttributes, limit, offset,
		)
		offset += limit

		req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			err = fmt.Errorf("failed to create request: %w", err)
			return
		}

		req.Header.Add("Accept", "application/ld+json")
		req.Header.Add("Link", entities.LinkHeader)

		if tenant != entities.DefaultNGSITenant {
			req.Header.Add("NGSILD-Tenant", tenant)
		}

		logger.Debug().Msgf("calling %s", url)

		resp, err = httpClient.Do(req)
		if err != nil {
			err = fmt.Errorf("failed to send request: %w", err)
			return
		}
		defer resp.Body.Close()

		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			err = fmt.Errorf("failed to read response body: %w", err)
			return
		}

		if resp.StatusCode >= http.StatusBadRequest {
			reqbytes, _ := httputil.DumpRequest(req, false)
			respbytes, _ := httputil.DumpResponse(resp, false)

			logger.Error().Str("request", string(reqbytes)).Str("response", string(respbytes)).Msg("request failed")
			err = fmt.Errorf("request failed")
			return
		}

		if resp.StatusCode != http.StatusOK {
			contentType := resp.Header.Get("Content-Type")
			return 0, fmt.Errorf("context source returned status code %d (content-type: %s, body: %s)", resp.StatusCode, contentType, string(respBody))
		}

		err = json.Unmarshal(respBody, &result)
		if err != nil {
			err = fmt.Errorf("failed to unmarshal response: %w", err)
			return
		}

		for _, e := range result {
			callback(e)
		}

		batchSize := len(result)
		count += batchSize

		if batchSize < limit {
			break
		}

		// Reset result size before continuing
		result = result[:0]
	}

	return
}
