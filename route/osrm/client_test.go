// © 2019-2022 nextmv.io inc. All rights reserved.
// nextmv.io, inc. CONFIDENTIAL
//
// This file includes unpublished proprietary source code of nextmv.io, inc.
// The copyright notice above does not evidence any actual or intended
// publication of such source code. Disclosure of this source code or any
// related proprietary information is strictly prohibited without the express
// written permission of nextmv.io, inc.

package osrm_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nextmv-io/sdk/route"
	"github.com/nextmv-io/sdk/route/osrm"
)

type testServer struct {
	s        *httptest.Server
	reqCount int
}

func newTestServer(t *testing.T, endpoint osrm.Endpoint) *testServer {
	ts := &testServer{}
	responseOk := ""
	if endpoint == osrm.TableEndpoint {
		responseOk = tableResponseOK
	} else {
		responseObject := osrm.RouteResponse{
			Code: "Ok",
			Routes: []osrm.Route{
				{
					Geometry: "mfp_I__vpAqJ`@wUrCa\\dCgGig@{DwW",
					Legs: []osrm.Leg{
						{
							Steps: []osrm.Step{
								{Geometry: "mfp_I__vpAWBQ@K@[BuBRgBLK@UBMMC?AA" +
									"KAe@FyBTC@E?IDKDA@K@]BUBSBA?E@E@A@KFUBK@mA" +
									"L{CZQ@qBRUBmAFc@@}@Fu@DG?a@B[@qAF}@JA?[D_" +
									"E`@SBO@ODA@UDA?]JC?uBNE?OAKA"},
								{Geometry: "yer_IcuupACa@AI]mCCUE[AK[iCWqB[{Bk" +
									"@sE_@_DAICSAOIm@AIQuACOQyAG[Gc@]wBw@aFKu@" +
									"y@oFCMAOIm@?K"},
								{Geometry: "}sr_IevwpA"},
							},
						},
					},
				},
			},
		}
		resp, err := json.Marshal(responseObject)
		if err != nil {
			t.Errorf("could not marshal response object, %v", err)
		}
		responseOk = string(resp)
	}
	ts.s = httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			ts.reqCount++
			_, err := io.WriteString(w, responseOk)
			if err != nil {
				t.Errorf("could not write resp: %v", err)
			}
		}),
	)
	return ts
}

func TestCacheHit(t *testing.T) {
	ts := newTestServer(t, osrm.TableEndpoint)
	defer ts.s.Close()

	c := osrm.DefaultClient(ts.s.URL, true)

	_, err := c.Get(ts.s.URL)
	if err != nil {
		t.Errorf("get failed: %v", err)
	}

	_, err = c.Get(ts.s.URL)
	if err != nil {
		t.Errorf("get failed: %v", err)
	}

	_, err = c.Get(ts.s.URL)
	if err != nil {
		t.Errorf("get failed: %v", err)
	}

	if ts.reqCount != 1 {
		t.Errorf("want: 1; got: %v", ts.reqCount)
	}
}

func TestCacheMiss(t *testing.T) {
	ts := newTestServer(t, osrm.TableEndpoint)
	defer ts.s.Close()

	c := osrm.DefaultClient(ts.s.URL, false)
	_, err := c.Get(ts.s.URL)
	if err != nil {
		t.Errorf("get failed: %v", err)
	}

	_, err = c.Get(ts.s.URL)
	if err != nil {
		t.Errorf("get failed: %v", err)
	}

	_, err = c.Get(ts.s.URL)
	if err != nil {
		t.Errorf("get failed: %v", err)
	}

	if ts.reqCount != 3 {
		t.Errorf("want: 3; got: %v", ts.reqCount)
	}
}

func TestMatrixCall(t *testing.T) {
	ts := newTestServer(t, osrm.TableEndpoint)
	defer ts.s.Close()

	c := osrm.DefaultClient(ts.s.URL, true)
	m, err := osrm.DurationMatrix(c, []route.Point{{0, 0}, {1, 1}}, 0)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if v := m.Cost(0, 1); v != 17699.1 {
		t.Errorf("want: 0; got: %v", v)
	}
}

func TestPolylineCall(t *testing.T) {
	ts := newTestServer(t, osrm.RouteEndpoint)
	defer ts.s.Close()

	c := osrm.DefaultClient(ts.s.URL, true)

	polyline, polyLegs, err := osrm.Polyline(
		c,
		[]route.Point{
			{13.388860, 52.517037},
			{13.397634, 52.529407},
		},
	)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	println(polyline)
	println(polyLegs)
}

const tableResponseOK = `{
	"code": "Ok",
	"sources": [{
		"hint": "",
		"distance": 9.215349,
		"name": "",
		"location": [-105.050583, 39.762548]
	}, {
		"hint": "",
		"distance": 11740767.450958,
		"name": "Prairie Hill Road",
		"location": [-104.095128, 38.21453]
	}],
	"destinations": [{
		"hint": "",
		"distance": 9.215349,
		"name": "",
		"location": [-105.050583, 39.762548]
	}, {
		"hint": "",
		"distance": 11740767.450958,
		"name": "Prairie Hill Road",
		"location": [-104.095128, 38.21453]
	}],
	"durations": [
		[0, 17699.1],
		[17732.3, 0]
	],
	"distances": [
		[0, 245976.4],
		[245938.6, 0]
	]
}`
