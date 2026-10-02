package nacos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// newV1CatalogStatisticsServer serves a v1 catalog service list whose entries carry
// the pojo.ServiceView count fields (clusterCount/ipCount/healthyInstanceCount).
func newV1CatalogStatisticsServer(t *testing.T, serviceList []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case nacosV3ReadinessPath:
			http.NotFound(w, r)
		case "/v1/console/namespaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []any{}})
		case "/v1/ns/catalog/services":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count":       len(serviceList),
				"serviceList": serviceList,
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestListServicesWithStatisticsParsesCatalogCounts(t *testing.T) {
	server := newV1CatalogStatisticsServer(t, []map[string]any{
		{"name": "orders", "groupName": "DEFAULT_GROUP", "clusterCount": 2, "ipCount": 5, "healthyInstanceCount": 4},
		{"name": "payments", "groupName": "DEFAULT_GROUP", "clusterCount": 1, "ipCount": 3, "healthyInstanceCount": 0},
	})
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID:    "dev",
		PageNo:         1,
		PageSize:       20,
		WithStatistics: true,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if !page.StatisticsAvailable {
		t.Fatal("StatisticsAvailable = false, want true for a catalog response carrying counts")
	}
	if page.StatisticsFamily != "v1" {
		t.Fatalf("StatisticsFamily = %q, want v1", page.StatisticsFamily)
	}
	if len(page.Services) != 2 {
		t.Fatalf("len(Services) = %d, want 2", len(page.Services))
	}
	first := page.Services[0]
	if first.Name != "orders" || first.GroupName != "DEFAULT_GROUP" {
		t.Fatalf("Services[0] identity = %#v", first)
	}
	if first.InstanceCount != 5 || first.HealthyInstanceCount != 4 {
		t.Fatalf("Services[0] counts = %#v, want instance=5 healthy=4", first)
	}
	// A reported zero is a measurement, not a missing field.
	second := page.Services[1]
	if second.HealthyInstanceCount != 0 || !second.StatisticsAvailable {
		t.Fatalf("Services[1] = %#v, want healthy=0 with StatisticsAvailable", second)
	}
	// ServiceNames keeps its existing contract and index alignment.
	if len(page.ServiceNames) != 2 || page.ServiceNames[0] != "DEFAULT_GROUP@@orders" {
		t.Fatalf("ServiceNames = %#v", page.ServiceNames)
	}
}

// A response that reports all-zero counts still counts as "reported": the keys are
// present, so the values are real measurements rather than absent fields.
func TestListServicesWithStatisticsTreatsPresentZeroKeysAsReported(t *testing.T) {
	server := newV1CatalogStatisticsServer(t, []map[string]any{
		{"name": "idle", "groupName": "DEFAULT_GROUP", "clusterCount": 0, "ipCount": 0, "healthyInstanceCount": 0},
	})
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID:    "dev",
		PageNo:         1,
		PageSize:       20,
		WithStatistics: true,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if !page.StatisticsAvailable {
		t.Fatal("StatisticsAvailable = false, want true: the server sent the count keys")
	}
	if len(page.Services) != 1 || page.Services[0].HealthyInstanceCount != 0 {
		t.Fatalf("Services = %#v", page.Services)
	}
}

// The v2 single-group route returns bare service names. Requesting statistics must
// report them as unavailable instead of fabricating zeros.
func TestListServicesWithStatisticsUnavailableForBareNameResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case nacosV3ReadinessPath:
			http.NotFound(w, r)
		case routesForNacosAPI(nacosAPIV2).namespaceList:
			writeNacosResult(w, nacosAPIV2, []any{})
		case routesForNacosAPI(nacosAPIV1).serviceList:
			// Catalog route: entries carry no count keys at all.
			writeNacosJSON(w, map[string]any{
				"count": 1,
				"serviceList": []any{
					map[string]any{"name": "orders", "groupName": "DEFAULT_GROUP"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID:    "dev",
		PageNo:         1,
		PageSize:       20,
		WithStatistics: true,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if page.StatisticsAvailable {
		t.Fatal("StatisticsAvailable = true, want false when the response has no count keys")
	}
	if len(page.Services) != 0 {
		t.Fatalf("Services = %#v, want empty when statistics are unavailable", page.Services)
	}
	if len(page.ServiceNames) != 1 {
		t.Fatalf("ServiceNames = %#v, want the name list unaffected", page.ServiceNames)
	}
}

// Without the opt-in the page must stay byte-for-byte backward compatible.
func TestListServicesWithoutStatisticsOmitsSummary(t *testing.T) {
	server := newV1CatalogStatisticsServer(t, []map[string]any{
		{"name": "orders", "groupName": "DEFAULT_GROUP", "clusterCount": 2, "ipCount": 5, "healthyInstanceCount": 4},
	})
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID: "dev", PageNo: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if page.StatisticsAvailable || len(page.Services) != 0 || page.StatisticsFamily != "" {
		t.Fatalf("page = %#v, want no statistics when not requested", page)
	}
	if len(page.ServiceNames) != 1 {
		t.Fatalf("ServiceNames = %#v", page.ServiceNames)
	}
}

func TestResponseReportsEveryServiceStatisticsDetectsAnyKey(t *testing.T) {
	for _, key := range serviceItemPayloadKeys {
		raw := []map[string]json.RawMessage{
			{"name": json.RawMessage(`"orders"`), key: json.RawMessage(`0`)},
		}
		if !responseReportsEveryServiceStatistics(raw) {
			t.Fatalf("responseReportsEveryServiceStatistics did not detect key %q", key)
		}
	}
	bare := []map[string]json.RawMessage{{"name": json.RawMessage(`"orders"`)}}
	if responseReportsEveryServiceStatistics(bare) {
		t.Fatal("responseReportsEveryServiceStatistics = true for a response with no count keys")
	}
	if responseReportsEveryServiceStatistics(nil) {
		t.Fatal("responseReportsEveryServiceStatistics(nil) = true")
	}
}

// The projection is a flat slice behind one flag, so a response that reports counts for
// only some of its services cannot be described per row. Availability must go false: the
// alternative renders the silent rows as zero instances, i.e. a live service shown as down.
func TestResponseReportsEveryServiceStatisticsRejectsMixedPayload(t *testing.T) {
	mixed := []map[string]json.RawMessage{
		{"name": json.RawMessage(`"orders"`), "ipCount": json.RawMessage(`3`)},
		{"name": json.RawMessage(`"payments"`)},
	}
	if responseReportsEveryServiceStatistics(mixed) {
		t.Fatal("mixed payload reported as fully statistics-bearing, want false")
	}
}

func TestProjectServiceStatisticsSkipsBlankNames(t *testing.T) {
	projected := projectServiceStatistics([]nacosServiceItem{
		{Name: "orders", GroupName: "G1", IPCount: 2, HealthyInstanceCount: 1},
		{Name: "   ", GroupName: "G1"},
	}, true)
	if len(projected) != 1 || projected[0].Name != "orders" {
		t.Fatalf("projected = %#v, want only the named service", projected)
	}
	if projected[0].InstanceCount != 2 || projected[0].HealthyInstanceCount != 1 {
		t.Fatalf("projected[0] = %#v", projected[0])
	}
	if got := projectServiceStatistics([]nacosServiceItem{{Name: "a"}}, false); got != nil {
		t.Fatalf("projectServiceStatistics(_, false) = %#v, want nil", got)
	}
}

func TestFamilyNameLabels(t *testing.T) {
	cases := map[nacosAPIFamily]string{
		nacosAPIV1:      "v1",
		nacosAPIV2:      "v2",
		nacosAPIV3:      "v3",
		nacosAPIUnknown: "unknown",
	}
	for family, want := range cases {
		if got := familyName(family); got != want {
			t.Fatalf("familyName(%v) = %q, want %q", family, got, want)
		}
	}
}

func TestExtractRawServiceItemsHandlesBareNameFamilies(t *testing.T) {
	// v1 single-group responses return {"doms": [...]} — plain strings, no stats.
	domesBody := []byte(`{"count":1,"doms":["orders"]}`)
	if got := extractRawServiceItems(domesBody, nacosAPIV1, false, "G1"); got != nil {
		t.Fatalf("v1 doms extraction = %#v, want nil", got)
	}
	// v2 single-group responses return {"services": [...]} for the same reason.
	servicesBody := []byte(`{"count":1,"services":["orders"]}`)
	if got := extractRawServiceItems(servicesBody, nacosAPIV2, false, "G1"); got != nil {
		t.Fatalf("v2 services extraction = %#v, want nil", got)
	}
	// Malformed payloads degrade to "no statistics" rather than erroring out.
	if got := extractRawServiceItems([]byte(`not json`), nacosAPIV3, false, ""); got != nil {
		t.Fatalf("malformed extraction = %#v, want nil", got)
	}
}

func TestExtractRawServiceItemsReadsV3PageItems(t *testing.T) {
	body := []byte(`{"totalCount":1,"pageItems":[{"name":"orders","groupName":"G1","ipCount":3}]}`)
	got := extractRawServiceItems(body, nacosAPIV3, false, "")
	if len(got) != 1 {
		t.Fatalf("v3 extraction = %#v, want 1 item", got)
	}
	if _, ok := got[0]["ipCount"]; !ok {
		t.Fatal("v3 extraction lost ipCount")
	}
	if !responseReportsEveryServiceStatistics(got) {
		t.Fatal("v3 pageItems with ipCount should register as carrying statistics")
	}
}

/* It reports counts back through the same response shape the app layer relays. */
func TestServicePageJSONShape(t *testing.T) {
	page := ServicePage{
		Count:        1,
		ServiceNames: []string{"G1@@orders"},
		Services: []ServiceSummary{
			{Name: "orders", GroupName: "G1", InstanceCount: 3, HealthyInstanceCount: 2, StatisticsAvailable: true},
		},
		StatisticsAvailable: true,
		StatisticsFamily:    "v1",
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{`"services"`, `"statisticsAvailable"`, `"statisticsFamily"`, `"healthyInstanceCount"`} {
		if !strings.Contains(string(encoded), key) {
			t.Fatalf("encoded page missing %s: %s", key, encoded)
		}
	}
	// InstanceCount is relayed under the json name the frontend reads.
	if !strings.Contains(string(encoded), `"instanceCount"`) {
		t.Fatalf("encoded page missing instanceCount: %s", encoded)
	}
}

// The sidebar's group rows open the workbench with a group filter applied, which
// routes to the group-scoped list rather than the namespace catalog. That path used
// to drop the instance counts the scan had already fetched, leaving the health column
// permanently empty exactly where the feature is entered from.
func TestListServicesWithStatisticsCoversGroupScopedCatalog(t *testing.T) {
	server := newV1CatalogStatisticsServer(t, []map[string]any{
		{"name": "orders", "groupName": "ORDER_GROUP", "clusterCount": 1, "ipCount": 4, "healthyInstanceCount": 3},
		{"name": "payments", "groupName": "PAY_GROUP", "clusterCount": 1, "ipCount": 2, "healthyInstanceCount": 2},
	})
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID:    "dev",
		GroupName:      "ORDER_GROUP",
		PageNo:         1,
		PageSize:       20,
		WithStatistics: true,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if !page.StatisticsAvailable {
		t.Fatal("StatisticsAvailable = false, want true for a group-scoped catalog request")
	}
	if len(page.Services) != 1 {
		t.Fatalf("Services = %#v, want only the requested group", page.Services)
	}
	if page.Services[0].Name != "orders" || page.Services[0].HealthyInstanceCount != 3 {
		t.Fatalf("Services[0] = %#v, want orders with 3 healthy instances", page.Services[0])
	}
	if page.StatisticsFamily != "v1" {
		t.Fatalf("StatisticsFamily = %q, want v1", page.StatisticsFamily)
	}
}

// Group-scoped reads without the opt-in keep using the bare-name endpoint and must
// stay free of the new fields.
func TestListServicesGroupScopedWithoutStatisticsOmitsSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case nacosV3ReadinessPath:
			http.NotFound(w, r)
		case routesForNacosAPI(nacosAPIV1).namespaceList:
			writeNacosJSON(w, map[string]any{"code": 200, "data": []any{}})
		case routesForNacosAPI(nacosAPIV1).serviceListByGroup:
			writeNacosJSON(w, map[string]any{
				"count": 1,
				"doms":  []any{"orders"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID: "dev",
		GroupName:   "ORDER_GROUP",
		PageNo:      1,
		PageSize:    20,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if page.StatisticsAvailable || len(page.Services) != 0 {
		t.Fatalf("page = %#v, want no statistics without the opt-in", page)
	}
	if len(page.ServiceNames) != 1 {
		t.Fatalf("ServiceNames = %#v, want the name list unaffected", page.ServiceNames)
	}
}

// newPagedGroupCatalogServer serves a v1 catalog whose group has exactly `total`
// services spread over pages of maxServicePageSize, followed by one empty trailing
// page — the shape a group whose size is an exact multiple of the page size produces.
func newPagedGroupCatalogServer(t *testing.T, group string, total int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case nacosV3ReadinessPath:
			http.NotFound(w, r)
		case routesForNacosAPI(nacosAPIV1).namespaceList:
			writeNacosJSON(w, map[string]any{"code": 200, "data": []any{}})
		case routesForNacosAPI(nacosAPIV1).serviceList:
			pageNo, _ := strconv.Atoi(r.URL.Query().Get("pageNo"))
			start := (pageNo - 1) * maxServicePageSize
			end := start + maxServicePageSize
			if end > total {
				end = total
			}
			list := make([]map[string]any, 0, maxServicePageSize)
			for i := start; i < end; i++ {
				list = append(list, map[string]any{
					"name":                 "svc-" + strconv.Itoa(i),
					"groupName":            group,
					"clusterCount":         1,
					"ipCount":              3,
					"healthyInstanceCount": 3,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"count": total, "serviceList": list})
		default:
			http.NotFound(w, r)
		}
	}))
}

// A group whose service count is an exact multiple of the page size ends on a
// legitimately empty page. The trailing page must not be read as "this server does
// not report statistics": that blanked the health column for the whole group.
func TestListServicesStatisticsSurviveEmptyTrailingPage(t *testing.T) {
	server := newPagedGroupCatalogServer(t, "ORDER_GROUP", maxServicePageSize)
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID:    "dev",
		GroupName:      "ORDER_GROUP",
		PageNo:         1,
		PageSize:       20,
		WithStatistics: true,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if !page.StatisticsAvailable {
		t.Fatal("StatisticsAvailable = false, want true: the empty trailing page carries no verdict")
	}
	if page.Count != maxServicePageSize {
		t.Fatalf("Count = %d, want %d", page.Count, maxServicePageSize)
	}
	if len(page.Services) != 20 {
		t.Fatalf("len(Services) = %d, want the page slice of 20", len(page.Services))
	}
	if page.Services[0].HealthyInstanceCount != 3 {
		t.Fatalf("Services[0] = %#v, want the reported counts", page.Services[0])
	}
}

// Control for the empty-trailing-page case: a non-multiple page count ends on a
// populated page and must keep statistics either way.
func TestListServicesStatisticsSurviveShortFinalPage(t *testing.T) {
	server := newPagedGroupCatalogServer(t, "ORDER_GROUP", maxServicePageSize-1)
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID:    "dev",
		GroupName:      "ORDER_GROUP",
		PageNo:         1,
		PageSize:       20,
		WithStatistics: true,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if !page.StatisticsAvailable {
		t.Fatal("StatisticsAvailable = false, want true for a short final page")
	}
	if page.Count != maxServicePageSize-1 {
		t.Fatalf("Count = %d, want %d", page.Count, maxServicePageSize-1)
	}
}

// A page that mixes items with and without the count keys is still ambiguous per row,
// and the flat projection cannot describe that. Availability must go false rather than
// render the silent rows as zero instances (a live service painted as down).
func TestListServicesStatisticsUnavailableForMixedPage(t *testing.T) {
	server := newV1CatalogStatisticsServer(t, []map[string]any{
		{"name": "orders", "groupName": "DEFAULT_GROUP", "ipCount": 3, "healthyInstanceCount": 3},
		{"name": "payments", "groupName": "DEFAULT_GROUP"},
	})
	defer server.Close()

	client := connectAPIVersionTestClient(t, server)
	defer client.Close()

	page, err := client.ListServices(context.Background(), ServiceQuery{
		NamespaceID:    "dev",
		PageNo:         1,
		PageSize:       20,
		WithStatistics: true,
	})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if page.StatisticsAvailable {
		t.Fatal("StatisticsAvailable = true, want false when only some items report counts")
	}
	if len(page.ServiceNames) != 2 {
		t.Fatalf("ServiceNames = %#v, want the name list unaffected", page.ServiceNames)
	}
}
