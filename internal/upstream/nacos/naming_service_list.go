package nacos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Service listing: the read path behind the sidebar group tree and the workbench
// service list. Split out of naming.go, which is far past the file-size limit and
// must not grow.

// nacosServiceItem is one entry of a service-list response. The count fields are
// optional: Nacos v1 CatalogController.listDetail (withInstances=false) returns
// pojo.ServiceView, which carries clusterCount/ipCount/healthyInstanceCount, while
// the v2/v3 shapes return bare service names. Whether the response really carried
// them is decided by responseReportsEveryServiceStatistics, so callers never see
// fabricated zeros.
type nacosServiceItem struct {
	Name                 string `json:"name"`
	GroupName            string `json:"groupName"`
	IPCount              int    `json:"ipCount"`
	HealthyInstanceCount int    `json:"healthyInstanceCount"`
}

// serviceItemPayloadKeys are the JSON keys that indicate a service-list response
// carries per-service instance statistics. Used to tell "the API family does not
// report counts" apart from "every service happens to have zero instances".
var serviceItemPayloadKeys = []string{"clusterCount", "ipCount", "healthyInstanceCount"}

// responseReportsEveryServiceStatistics reports whether every raw service item in the
// response exposed at least one statistics key. The check runs on the raw decoded items
// because a service with genuinely zero instances is indistinguishable from an absent
// field once unmarshalled into ints.
//
// It asks for "every" rather than "any" on purpose. The projection is a flat slice with a
// single availability flag, so a response mixing items with and without the keys cannot be
// described per row: claiming availability would render the key-less items as zero
// instances, i.e. a healthy service shown as down. Reporting unavailable instead only
// hides the counts — the safe direction. Same-family responses are uniform in practice,
// so this decides nothing more than a degenerate case.
//
// An empty response reports nothing, i.e. false. Callers walking several pages must not
// fold that verdict in: the trailing page of an exact multiple of the page size is
// legitimately empty, and treating it as "this server does not report statistics" would
// blank the counts for the whole group. See foldPageStatistics.
func responseReportsEveryServiceStatistics(items []map[string]json.RawMessage) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		reported := false
		for _, key := range serviceItemPayloadKeys {
			if _, ok := item[key]; ok {
				reported = true
				break
			}
		}
		if !reported {
			return false
		}
	}
	return true
}

// foldPageStatistics feeds one page of a multi-page scan into a running availability
// verdict, ignoring empty pages.
//
// Empty pages carry no items to inspect, so responseReportsEveryServiceStatistics calls
// them "no statistics" — correct in isolation, wrong when folded: a group whose service
// count is an exact multiple of the page size ends on a legitimately empty page, and
// folding that verdict would blank the counts for every service in the group. Only pages
// with items are allowed to speak.
//
// The verdict only ever narrows: one page that fails to report statistics is enough to
// make the whole scan unreliable, and a later well-formed page cannot undo that.
func foldPageStatistics(items []map[string]json.RawMessage, reportsSoFar bool) bool {
	if len(items) == 0 {
		return reportsSoFar
	}
	return reportsSoFar && responseReportsEveryServiceStatistics(items)
}

// familyName gives the API family a stable diagnostic label.
func familyName(family nacosAPIFamily) string {
	switch family {
	case nacosAPIV1:
		return "v1"
	case nacosAPIV2:
		return "v2"
	case nacosAPIV3:
		return "v3"
	default:
		return "unknown"
	}
}

// extractRawServiceItems pulls the undecoded service entries out of a service-list
// response so callers can inspect which JSON keys the server actually sent. The
// shape depends on the API family exactly like the typed decode below; entries that
// are plain strings (v1/v2 single-group responses) carry no statistics by
// construction and are returned as nil.
func extractRawServiceItems(
	data []byte,
	family nacosAPIFamily,
	useCatalog bool,
	groupName string,
) []map[string]json.RawMessage {
	// Branches mirror the typed decode in ListServices.
	if family == nacosAPIV3 {
		var payload struct {
			PageItems []map[string]json.RawMessage `json:"pageItems"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		return payload.PageItems
	}
	if useCatalog {
		var payload struct {
			ServiceList []map[string]json.RawMessage `json:"serviceList"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil
		}
		return payload.ServiceList
	}
	if groupName != "" {
		// v1 /v1/ns/service/list ("doms") and v2 /v2/ns/service/list ("services")
		// return bare name arrays: no per-service statistics by construction.
		return nil
	}
	var payload struct {
		ServiceList []map[string]json.RawMessage `json:"serviceList"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return payload.ServiceList
}

// projectServiceStatistics builds the optional per-service summary slice for a
// page, index-aligned with the emitted service names.
func projectServiceStatistics(
	services []nacosServiceItem,
	statsAvailable bool,
) []ServiceSummary {
	if !statsAvailable {
		return nil
	}
	summaries := make([]ServiceSummary, 0, len(services))
	for _, service := range services {
		name := strings.TrimSpace(service.Name)
		if name == "" {
			continue
		}
		summaries = append(summaries, ServiceSummary{
			Name:                 name,
			GroupName:            normalizeServiceGroup(service.GroupName),
			InstanceCount:        service.IPCount,
			HealthyInstanceCount: service.HealthyInstanceCount,
			StatisticsAvailable:  true,
		})
	}
	if len(summaries) == 0 {
		return nil
	}
	return summaries
}

type nacosV3ServicePage struct {
	TotalCount     int64              `json:"totalCount"`
	PageNumber     int                `json:"pageNumber"`
	PagesAvailable int                `json:"pagesAvailable"`
	PageItems      []nacosServiceItem `json:"pageItems"`
}

// ListServices lists service names under a namespace.
func (c *ClientImpl) ListServices(ctx context.Context, query ServiceQuery) (*ServicePage, error) {
	family := c.currentAPIFamily()
	pageNo := query.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = defaultServicePageSize
	}
	if pageSize > maxServicePageSize {
		pageSize = maxServicePageSize
	}

	groupName := strings.TrimSpace(query.GroupName)
	serviceName := strings.TrimSpace(query.ServiceName)
	// The group-scoped v1/v2 list (/v1/ns/service/list, "doms") returns bare names with
	// no instance counts, so a statistics request must go through the catalog instead.
	// This is the path the sidebar takes: its group rows open the workbench with that
	// group pre-filtered, so without this the health column would be empty exactly
	// where the feature is entered from.
	if (family == nacosAPIV1 || family == nacosAPIV2) && groupName != "" && (serviceName != "" || query.WithStatistics) {
		return c.listCatalogServicesByExactGroup(ctx, query, groupName, serviceName, pageNo, pageSize)
	}
	if family == nacosAPIV3 && groupName != "" {
		return c.listV3ServicesByExactGroup(ctx, query, groupName, serviceName, pageNo, pageSize)
	}

	params := url.Values{}
	params.Set("pageNo", strconv.Itoa(pageNo))
	params.Set("pageSize", strconv.Itoa(pageSize))
	params.Set("namespaceId", normalizeNamespaceID(query.NamespaceID))
	routes := c.currentAPIRoutes()
	apiPath := routes.serviceList
	useCatalog := (family == nacosAPIV1 || family == nacosAPIV2) && (groupName == "" || serviceName != "")
	if useCatalog {
		// Nacos 2.x has no cross-group v2 service list, but retains v1 Catalog.
		apiPath = routesForNacosAPI(nacosAPIV1).serviceList
		params.Set("serviceNameParam", serviceName)
		params.Set("groupNameParam", "")
	} else if family == nacosAPIV1 || family == nacosAPIV2 {
		apiPath = routes.serviceListByGroup
		params.Set("groupName", normalizeServiceGroup(groupName))
		params.Set("serviceNameParam", serviceName)
	} else {
		params.Set("serviceNameParam", serviceName)
		params.Set("groupNameParam", groupName)
	}

	body, status, err := c.doRequest(ctx, http.MethodGet, apiPath, params, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, localizedNacosBackendError("nacos.backend.error.http_status", map[string]any{
			"status": status,
			"body":   truncateForError(string(body)),
		})
	}

	data, err := unwrapNacosResult(body)
	if err != nil {
		return nil, err
	}
	var count int64
	var services []nacosServiceItem
	// rawItems retains the undecoded service entries so we can tell "this API
	// family does not report instance statistics" apart from "the reported counts
	// happen to be zero". Only captured when the caller asked for statistics.
	var rawItems []map[string]json.RawMessage
	if query.WithStatistics {
		rawItems = extractRawServiceItems(data, family, useCatalog, groupName)
	}
	if family == nacosAPIV3 {
		var payload nacosV3ServicePage
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, localizedNacosBackendError("nacos.backend.error.parse_services", map[string]any{
				"detail": err.Error(),
			})
		}
		count = payload.TotalCount
		services = payload.PageItems
	} else if useCatalog {
		var payload struct {
			Count       int64              `json:"count"`
			ServiceList []nacosServiceItem `json:"serviceList"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, localizedNacosBackendError("nacos.backend.error.parse_services", map[string]any{
				"detail": err.Error(),
			})
		}
		count = payload.Count
		services = payload.ServiceList
	} else if family == nacosAPIV1 && groupName != "" {
		var payload struct {
			Count int64    `json:"count"`
			Doms  []string `json:"doms"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, localizedNacosBackendError("nacos.backend.error.parse_services", map[string]any{
				"detail": err.Error(),
			})
		}
		count = payload.Count
		services = make([]nacosServiceItem, 0, len(payload.Doms))
		for _, name := range payload.Doms {
			services = append(services, nacosServiceItem{Name: name, GroupName: normalizeServiceGroup(groupName)})
		}
	} else if family == nacosAPIV2 && groupName != "" {
		var payload struct {
			Count    int64    `json:"count"`
			Services []string `json:"services"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, localizedNacosBackendError("nacos.backend.error.parse_services", map[string]any{
				"detail": err.Error(),
			})
		}
		count = payload.Count
		services = make([]nacosServiceItem, 0, len(payload.Services))
		for _, name := range payload.Services {
			services = append(services, nacosServiceItem{Name: name, GroupName: normalizeServiceGroup(groupName)})
		}
	} else {
		var payload struct {
			Count       int64              `json:"count"`
			ServiceList []nacosServiceItem `json:"serviceList"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, localizedNacosBackendError("nacos.backend.error.parse_services", map[string]any{
				"detail": err.Error(),
			})
		}
		count = payload.Count
		services = payload.ServiceList
	}
	if useCatalog && groupName != "" {
		filtered := make([]nacosServiceItem, 0, len(services))
		for _, service := range services {
			if strings.TrimSpace(service.GroupName) == groupName {
				filtered = append(filtered, service)
			}
		}
		services = filtered
		count = int64(len(services))
	}

	names := make([]string, 0, len(services))
	for _, service := range services {
		name := qualifyServiceName(service.Name, service.GroupName)
		if name != "" {
			names = append(names, name)
		}
	}
	page := &ServicePage{
		Count:        count,
		ServiceNames: names,
		PageNo:       pageNo,
		PageSize:     pageSize,
	}
	if query.WithStatistics {
		page.Services = projectServiceStatistics(services, responseReportsEveryServiceStatistics(rawItems))
		page.StatisticsAvailable = len(page.Services) > 0
		if page.StatisticsAvailable {
			page.StatisticsFamily = familyName(family)
		}
	}
	return page, nil
}

func (c *ClientImpl) listCatalogServicesByExactGroup(
	ctx context.Context,
	query ServiceQuery,
	groupName string,
	serviceName string,
	pageNo int,
	pageSize int,
) (*ServicePage, error) {
	namespaceID := query.NamespaceID
	groupName = normalizeServiceGroup(groupName)
	matched := make([]nacosServiceItem, 0)
	// The catalog is expected to report counts, but the flag is still derived from the
	// payload rather than assumed, so a gateway that strips the keys degrades to "no
	// statistics" instead of fabricating zeros.
	reportsStatistics := true
	for remotePageNo := 1; ; remotePageNo++ {
		params := url.Values{}
		params.Set("pageNo", strconv.Itoa(remotePageNo))
		params.Set("pageSize", strconv.Itoa(maxServicePageSize))
		params.Set("namespaceId", normalizeNamespaceID(namespaceID))
		params.Set("serviceNameParam", serviceName)
		// Nacos validates this parameter as a plain value; regex anchors are rejected.
		params.Set("groupNameParam", groupName)

		body, status, err := c.doRequest(ctx, http.MethodGet, routesForNacosAPI(nacosAPIV1).serviceList, params, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, localizedNacosBackendError("nacos.backend.error.http_status", map[string]any{
				"status": status,
				"body":   truncateForError(string(body)),
			})
		}
		data, err := unwrapNacosResult(body)
		if err != nil {
			return nil, err
		}
		var payload struct {
			ServiceList []nacosServiceItem `json:"serviceList"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, localizedNacosBackendError("nacos.backend.error.parse_services", map[string]any{
				"detail": err.Error(),
			})
		}
		if query.WithStatistics {
			reportsStatistics = foldPageStatistics(extractRawServiceItems(data, nacosAPIV1, true, groupName), reportsStatistics)
		}
		for _, service := range payload.ServiceList {
			if strings.TrimSpace(service.GroupName) == groupName {
				matched = append(matched, service)
			}
		}
		if len(payload.ServiceList) < maxServicePageSize {
			break
		}
	}

	start := (pageNo - 1) * pageSize
	if start > len(matched) {
		start = len(matched)
	}
	end := min(start+pageSize, len(matched))
	names := make([]string, 0, end-start)
	for _, service := range matched[start:end] {
		if name := qualifyServiceName(service.Name, service.GroupName); name != "" {
			names = append(names, name)
		}
	}
	page := &ServicePage{
		Count:        int64(len(matched)),
		ServiceNames: names,
		PageNo:       pageNo,
		PageSize:     pageSize,
	}
	// This scan already fetched the catalog entries with their counts; without the
	// projection the service list would show an empty health column whenever it is
	// opened for a single group, which is the entry point the sidebar group rows use.
	if query.WithStatistics {
		page.Services = projectServiceStatistics(matched[start:end], reportsStatistics)
		page.StatisticsAvailable = len(page.Services) > 0
		if page.StatisticsAvailable {
			page.StatisticsFamily = familyName(nacosAPIV1)
		}
	}
	return page, nil
}

func (c *ClientImpl) listV3ServicesByExactGroup(
	ctx context.Context,
	query ServiceQuery,
	groupName string,
	serviceName string,
	pageNo int,
	pageSize int,
) (*ServicePage, error) {
	namespaceID := query.NamespaceID
	groupName = normalizeServiceGroup(groupName)
	matched := make([]nacosServiceItem, 0)
	// Derived from the payload, not assumed — see listCatalogServicesByExactGroup.
	reportsStatistics := true
	for remotePageNo := 1; ; remotePageNo++ {
		params := url.Values{}
		params.Set("pageNo", strconv.Itoa(remotePageNo))
		params.Set("pageSize", strconv.Itoa(maxServicePageSize))
		params.Set("namespaceId", normalizeNamespaceID(namespaceID))
		params.Set("serviceNameParam", serviceName)
		// Nacos validates this parameter as a plain value; regex escaping is rejected.
		params.Set("groupNameParam", groupName)

		body, status, err := c.doRequest(ctx, http.MethodGet, c.currentAPIRoutes().serviceListByGroup, params, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, localizedNacosBackendError("nacos.backend.error.http_status", map[string]any{
				"status": status,
				"body":   truncateForError(string(body)),
			})
		}
		data, err := unwrapNacosResult(body)
		if err != nil {
			return nil, err
		}
		var payload nacosV3ServicePage
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, localizedNacosBackendError("nacos.backend.error.parse_services", map[string]any{
				"detail": err.Error(),
			})
		}
		if query.WithStatistics {
			reportsStatistics = foldPageStatistics(extractRawServiceItems(data, nacosAPIV3, false, groupName), reportsStatistics)
		}
		for _, service := range payload.PageItems {
			if strings.TrimSpace(service.GroupName) == groupName {
				matched = append(matched, service)
			}
		}
		if payload.PagesAvailable <= remotePageNo {
			break
		}
	}

	start := (pageNo - 1) * pageSize
	if start > len(matched) {
		start = len(matched)
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	names := make([]string, 0, end-start)
	for _, service := range matched[start:end] {
		if name := qualifyServiceName(service.Name, service.GroupName); name != "" {
			names = append(names, name)
		}
	}
	page := &ServicePage{
		Count:        int64(len(matched)),
		ServiceNames: names,
		PageNo:       pageNo,
		PageSize:     pageSize,
	}
	// Same reason as listCatalogServicesByExactGroup: the group-scoped list is how
	// the sidebar opens the workbench, and it must not show an empty health column.
	if query.WithStatistics {
		page.Services = projectServiceStatistics(matched[start:end], reportsStatistics)
		page.StatisticsAvailable = len(page.Services) > 0
		if page.StatisticsAvailable {
			page.StatisticsFamily = familyName(nacosAPIV3)
		}
	}
	return page, nil
}
