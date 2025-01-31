package osrm

import (
	"context"
	// Ignore the gosec issue, see comments in the usage of sha1 down below.
	// G505 (CWE-327): Blocklisted import crypto/sha1: weak cryptographic primitive.
	/* #nosec */
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	lru "github.com/hashicorp/golang-lru"
	"github.com/nextmv-io/sdk/measure"
	polyline "github.com/twpayne/go-polyline"
)

// Endpoint defines the OSRM endpoint to be used.
type Endpoint string

const (
	// TableEndpoint is used to retrieve distance and duration matrices.
	TableEndpoint Endpoint = "table"
	// RouteEndpoint is used to retrieve polylines for a set of points.
	RouteEndpoint Endpoint = "route"
)

// Client represents an OSRM client.
type Client interface {
	// Table requests a distance and/or duration table from an OSRM server.
	Table(
		points []measure.Point,
		opts ...TableOptions,
	) (
		distance, duration [][]float64,
		err error,
	)
	// Get performs a GET against the OSRM server returning the response
	// body and an error.
	Get(uri string) ([]byte, error)
	// IgnoreEmpty removes empty / zero points from the request before sending
	// it to the OSRM server. The indices of the points will be maintained.
	// Distances / durations for these points will be set to 0.
	IgnoreEmpty(ignore bool)
	// SnapRadius limits snapping a point to the street network to given radius
	// in meters.
	// Setting the snap radius to a value = 0 results in an unlimited snapping
	// radius.
	SnapRadius(radius int) error
	// ScaleFactor is used in conjunction with duration calculations. Scales the
	// table duration values by this number. This does not affect distances.
	ScaleFactor(factor float64) error

	// MaxTableSize should be configured with the same value as the OSRM
	// server's max-table-size setting, default is 100
	MaxTableSize(size int) error

	// Polyline requests polylines for the given points. The first parameter
	// returns a polyline from start to end and the second parameter returns a
	// list of polylines, one per leg.
	Polyline(points []measure.Point) (string, []string, error)
}

// NewClient returns a new OSRM Client.
func NewClient(host string, opts ...ClientOption) Client {
	c := &client{
		host:         host,
		httpClient:   http.DefaultClient,
		snapRadius:   0,
		maxTableSize: 100,
		scaleFactor:  1.0,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// DefaultClient creates a new OSRM Client.
func DefaultClient(host string, useCache bool) Client {
	opts := []ClientOption{}
	if useCache {
		opts = append(opts, WithCache(100))
	}
	c := NewClient(host, opts...)

	return c
}

// A client makes requests to an OSRM server.
type client struct {
	httpClient   *http.Client
	cache        *lru.Cache
	host         string
	removeEmpty  bool
	snapRadius   int
	scaleFactor  float64
	maxTableSize int
	useCache     bool
}

func (c *client) IgnoreEmpty(ignore bool) {
	c.removeEmpty = ignore
}

func (c *client) SnapRadius(radius int) error {
	if radius < 0 {
		return errors.New("radius must be >= 0")
	}
	c.snapRadius = radius
	return nil
}

func (c *client) MaxTableSize(size int) error {
	if size < 1 {
		return errors.New("max table size must be > 0")
	}
	c.maxTableSize = size
	return nil
}

func (c *client) ScaleFactor(factor float64) error {
	if factor <= 0 {
		return errors.New("scale factor must be > 0")
	}
	c.scaleFactor = factor
	return nil
}

func handleErrorStatus(resp *http.Response) error {
	defer func() {
		// there is nothing we can really do with an error here.
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusBadRequest {
		errMsg := fmt.Sprintf("Error in input data when getting maps: %s", body)
		return NewError(errMsg, true)
	}
	return NewError(fmt.Sprintf("error response from OSRM: %s", body), false)
}

// get performs a GET.
func (c *client) get(uri string) (data []byte, err error) {
	var key string

	if c.useCache {
		// sha1 is used to shorten the key for faster cache lookup than directly using the lenthy uri as key.
		// The chance of hash colision is extremely low for sha1.
		// The cache is local to the user, which won't become a security threat even when key colides.
		// G401 (CWE-326): Use of weak cryptographic primitive.
		/* #nosec */
		key = fmt.Sprintf("%x", sha1.Sum([]byte(uri)))
		if result, ok := c.cache.Get(key); ok {
			if b, ok := result.([]byte); ok {
				return b, err
			}
		}
	}

	// convert host to URL
	h, err := url.Parse(c.host)
	if err != nil {
		return data, err
	}

	// convert uri to URL
	u, err := url.Parse(uri)
	if err != nil {
		return data, err
	}

	// safely join host and uri
	// http://example.com/foo
	u = h.ResolveReference(u)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet, u.String(), nil,
	)
	if err != nil {
		return data, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return data, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, handleErrorStatus(resp)
	}

	data, err = io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return data, err
	}

	if c.useCache {
		c.cache.Add(key, data)
	}

	err = resp.Body.Close()
	return data, err
}

func (c *client) Get(uri string) ([]byte, error) {
	return c.get(uri)
}

func (c *client) Table(points []measure.Point, opts ...TableOptions) (
	distances, durations [][]float64,
	err error,
) {
	cfg := &tableConfig{
		parallelRuns: 16,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	if len(points) == 0 {
		return nil, nil, fmt.Errorf("cannot request distances/durations for empty points")
	}

	// Remove empty points, if requested.
	originalLength := len(points)
	var deflatedIndices []int
	if c.removeEmpty {
		points, deflatedIndices = deflateZeroes(points)
	}

	// Creates paths with sources to make requests "by row".
	requests, err := c.tableRequests(cfg, points)
	if err != nil {
		return nil, nil, err
	}
	// Run parallel requests.
	out := make(chan result, len(requests))
	defer close(out)
	guard := make(chan struct{}, cfg.parallelRuns)
	defer close(guard)

	for _, req := range requests {
		go func(req request) {
			defer func() { <-guard }()
			guard <- struct{}{} // would block if guard channel is already filled
			body, err := c.get(req.path)
			if err != nil {
				out <- result{res: nil, err: err}
				return
			}

			var tableResp tableResponse
			if err := json.Unmarshal(body, &tableResp); err != nil {
				out <- result{res: nil, err: err}
				return
			}

			if c := tableResp.Code; c != "Ok" {
				fmtString := `expected "Ok" response code; got %q (%q)`
				out <- result{
					res: nil,
					err: fmt.Errorf(fmtString, c, tableResp.Message),
				}
				return
			}
			tableResp.row = req.row
			tableResp.column = req.column
			out <- result{res: &tableResp, err: nil}
		}(req)
	}

	// Empty chan to a list of responses.
	var responses []tableResponse
	var errs []error
	for i := 0; i < len(requests); i++ {
		r := <-out
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		responses = append(responses, *r.res)
	}

	if len(errs) > 0 {
		hasUserErrs := false
		errMsgs := make([]string, len(errs))

		for _, err := range errs {
			e, ok := errs[0].(Error)
			if ok && e.IsInputError() {
				hasUserErrs = true
			}
			errMsgs = append(errMsgs, err.Error())
		}
		errs := strings.Join(errMsgs, "\n")
		if hasUserErrs {
			return nil, nil, NewError(errs, true)
		}
		return nil, nil, NewError(errs, false)
	}

	// Stitch responses together.
	routeResp := mergeRequests(responses)

	// Reinflate the response matrix to the original size, if necessary.
	if c.removeEmpty {
		routeResp.Distances = inflateZeroes(routeResp.Distances, deflatedIndices, originalLength)
		routeResp.Durations = inflateZeroes(routeResp.Durations, deflatedIndices, originalLength)
	}

	return routeResp.Distances, routeResp.Durations, nil
}

var unroutablePoint = measure.Point{-143.292892, 37.683603}

func (c *client) tableRequests( //nolint:gocyclo
	config *tableConfig,
	points []measure.Point,
) ([]request, error) {
	// Turn points slice into OSRM-friendly semicolon-delimited point pairs
	// []{{1,2}, {3,4}} => "1,2;3,4"
	convertedPoints := make([][]float64, len(points))
	for i, point := range handleUnroutablePoints(points) {
		convertedPoints[i] = []float64{
			point[0], point[1],
		}
	}
	pointChunks := chunkBy(convertedPoints, c.maxTableSize)
	requests := make([]request, 0)
	for p1, pointChunk1 := range pointChunks {
		for p2, pointChunk2 := range pointChunks {
			resultingChunk := make([][]float64, len(pointChunk1)+len(pointChunk2))
			copy(resultingChunk, pointChunk1)
			copy(resultingChunk[len(pointChunk1):], pointChunk2)

			// Create points string and assemble path.
			sb := strings.Builder{}
			for i, point := range resultingChunk {
				sb.WriteString(fmt.Sprintf("%f,%f", point[0], point[1]))
				if i != len(resultingChunk)-1 {
					sb.WriteString(";")
				}
			}
			path, err := getPath(TableEndpoint, sb.String())
			if err != nil {
				return nil, err
			}

			// Determine the information to return. If neither distance nor
			// duration is requested, we return both.
			isDefault := !config.withDistance && !config.withDuration
			annotations := []string{}
			if isDefault || config.withDuration {
				annotations = append(annotations, "duration")
			}
			if isDefault || config.withDistance {
				annotations = append(annotations, "distance")
			}

			// The OSRM server will error when annotations are properly escaped, making
			// url.Values{} nonviable
			if len(annotations) >= 1 {
				path += "?annotations="
				path += strings.Join(annotations, ",")
			}

			if config.withApproachCurb {
				path += "&approaches="
				approaches := make([]string, len(resultingChunk))
				for i := 0; i < len(approaches); i++ {
					approaches[i] = "curb"
				}
				path += strings.Join(approaches, ";")
			}

			if len(config.withExclude) > 0 {
				path += "&exclude="
				path += strings.Join(config.withExclude, ",")
			}

			// Set scale factor. This only has an effect on durations.
			if c.scaleFactor != 1.0 {
				path += fmt.Sprintf("&scale_factor=%f", c.scaleFactor)
			}

			if c.snapRadius > 0 {
				// Set snap radius for points
				path += "&radiuses="
				radiuses := make([]string, len(resultingChunk))
				for i := 0; i < len(radiuses); i++ {
					radiuses[i] = strconv.Itoa(c.snapRadius)
				}
				path += strings.Join(radiuses, ";")
			}

			indices := make([]string, len(resultingChunk))
			for i := 0; i < len(indices); i++ {
				indices[i] = strconv.Itoa(i)
			}

			requests = append(requests,
				request{
					row:    p1,
					column: p2,
					path: path +
						"&sources=" + strings.Join(indices[:len(pointChunk1)], ";") +
						"&destinations=" + strings.Join(indices[len(pointChunk1):], ";"),
				},
			)
		}
	}
	return requests, nil
}

// result gathers a response and possible error from concurrent requests.
type result struct {
	res *tableResponse
	err error
}

// request holds a request and the request index for later stitching.
type request struct {
	path   string
	row    int
	column int
}

// tableResponse holds the tableResponse from the OSRM server.
type tableResponse struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Distances [][]float64 `json:"distances"`
	Durations [][]float64 `json:"durations"`
	row       int
	column    int
}

// TableOptions is a function that configures a tableConfig.
type TableOptions func(*tableConfig)

// tableConfig defines options for the table configuration.
type tableConfig struct {
	withDistance     bool
	withDuration     bool
	parallelRuns     int
	withApproachCurb bool
	withExclude      []string
}

// WithDuration returns a TableOptions function for composing a tableConfig with
// duration data enabled, telling the OSRM server to include duration data in
// the response table data.
func WithDuration() TableOptions {
	return func(c *tableConfig) {
		c.withDuration = true
	}
}

// WithDistance returns a TableOptions function for composing a tableConfig with
// distance data enabled, telling the OSRM server to include distance data in
// the response table data.
func WithDistance() TableOptions {
	return func(c *tableConfig) {
		c.withDistance = true
	}
}

// WithApproachCurb returns a TableOptions func for a tableConfig with the
// approach curb set.
func WithApproachCurb() TableOptions {
	return func(c *tableConfig) {
		c.withApproachCurb = true
	}
}

// WithExclude returns a TableOptions func for a tableConfig with the exclude
// parameter set.
func WithExclude(exclude []string) TableOptions {
	return func(c *tableConfig) {
		c.withExclude = exclude
	}
}

// ClientOption can pass options to be used with an OSRM client.
type ClientOption func(*client)

// WithClientTransport overwrites the RoundTripper used by the internal
// http.Client.
func WithClientTransport(rt http.RoundTripper) ClientOption {
	if rt == nil {
		rt = http.DefaultTransport
	}

	return func(c *client) {
		c.httpClient.Transport = rt
	}
}

// WithCache configures the maximum number of results cached.
func WithCache(maxItems int) ClientOption {
	return func(c *client) {
		c.useCache = true

		cache, _ := lru.New(maxItems)
		c.cache = cache
	}
}

// ParallelRuns set the number of parallel calls to the OSRM server. If 0 is
// passed, the default value of 16 will be used.
func ParallelRuns(runs int) TableOptions {
	return func(c *tableConfig) {
		if runs > 0 {
			c.parallelRuns = runs
		}
	}
}

// Creates the points parameters for an OSRM request.
func pointsParameters(points []measure.Point) []string {
	// Turn points slice into OSRM-friendly semicolon-delimited point pairs
	// []{{1,2}, {3,4}} => "1,2;3,4"
	pointStrings := []string{}
	points = handleUnroutablePoints(points)
	for _, point := range points {
		pointStrings = append(pointStrings, fmt.Sprintf("%f,%f", point[0], point[1]))
	}
	return pointStrings
}

// sets nil values to an unroutable point.
func handleUnroutablePoints(in []measure.Point) (out []measure.Point) {
	out = make([]measure.Point, len(in))
	for i, point := range in {
		if len(point) == 2 {
			out[i] = in[i]
		} else {
			out[i] = unroutablePoint
		}
	}
	return out
}

// Creates the points parameter for an OSRM request.
func pointsParameter(points []measure.Point) string {
	return strings.Join(pointsParameters(points), ";")
}

// RouteResponse holds the route response from the OSRM server.
type RouteResponse struct {
	Code    string  `json:"code"`
	Routes  []Route `json:"routes"`
	Message string  `json:"message"`
}

// Route partially represents the OSRM Route object.
type Route struct {
	Geometry string `json:"geometry"`
	Legs     []Leg  `json:"legs"`
}

// Leg partially represents the OSRM Leg object.
type Leg struct {
	Steps []Step `json:"steps"`
}

// Step partially represents the OSRM Step object.
type Step struct {
	Geometry string `json:"geometry"`
}

// Creates polylines for the given points. First return parameter is a polyline
// from start to end, second parameter is a list of polylines per leg in the
// route.
func (c *client) Polyline(points []measure.Point) (string, []string, error) {
	if len(points) == 0 {
		return "", []string{}, fmt.Errorf("cannot create polyline for empty points")
	}

	// Turn points slice into OSRM-friendly semicolon-delimited point pairs
	// []{{1,2}, {3,4}} => "1,2;3,4"
	pointsParameter := pointsParameter(points)

	path, err := getPath(RouteEndpoint, pointsParameter)
	if err != nil {
		return "", []string{}, err
	}

	// Get the simplified overview and single steps but no verbose annotations.
	path += "?overview=simplified&steps=true&annotations=false" +
		"&continue_straight=false"

	body, err := c.get(path)
	if err != nil {
		return "", []string{}, err
	}

	var routeResp RouteResponse
	if err := json.Unmarshal(body, &routeResp); err != nil {
		return "", []string{}, err
	}

	if routeResp.Code != "Ok" {
		return "", []string{}, fmt.Errorf(
			`expected "Ok" response code; got %q (%q)`,
			routeResp.Code,
			routeResp.Message,
		)
	}

	// The fist route is the calculated route. Other routes are alternative
	// routes that can be calculated but are not calculated in our case.
	route := routeResp.Routes[0]

	decodedLegs := make([][][]float64, len(points)-1)

	// Loop over every step in every leg and stich the decoded steps together.
	for i, leg := range route.Legs {
		for _, steps := range leg.Steps {
			buf := []byte(steps.Geometry)
			coords, _, err := polyline.DecodeCoords(buf)
			if err != nil {
				return "", []string{}, err
			}
			decodedLegs[i] = append(decodedLegs[i], coords...)
		}
	}

	legs := make([]string, len(points)-1)
	for i, leg := range decodedLegs {
		legs[i] = string(polyline.EncodeCoords(leg))
	}

	return route.Geometry, legs, nil
}

// Creates the path to the given endpoint including the given points.
func getPath(endpoint Endpoint, pointsParameter string) (string, error) {
	u, err := url.Parse(fmt.Sprintf("/%s/v1/driving/", string(endpoint)))
	if err != nil {
		return "", err
	}

	pointsURL, err := url.Parse(pointsParameter)
	if err != nil {
		return "", err
	}

	u = u.ResolveReference(pointsURL)
	return u.String(), nil
}

// chunkBy converts a slice of things into smaller slices of a given max size.
func chunkBy[T any](items []T, chunkSize int) (chunks [][]T) {
	chunks = make([][]T, 0, (len(items)/chunkSize)+1)
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:],
			append(chunks, items[0:chunkSize:chunkSize])
	}
	return append(chunks, items)
}

// mergeRequests stitches the given responses (and their matrices) together. The
// input responses can be in arbitrary order, but will be overwritten in the
// process.
func mergeRequests(responses []tableResponse) tableResponse {
	// Sort the responses by index, that orders the row packs correctly.
	sort.Slice(responses, func(i, j int) bool {
		if responses[i].row == responses[j].row {
			return responses[i].column < responses[j].column
		}
		return responses[i].row < responses[j].row
	})

	// --> Stitch distance matrices together
	// Expects submatrices of the following structure:
	//  a a b b c
	//  a a b b c
	//  d d e e f
	//  d d e e f
	//  g g h h i
	// The submatrices A, B, C, D, E, F of size 2x2 or less (at the edges) are
	// merged into a single matrix of size 5x5.
	// Furthermore, distance and duration matrices will be handled separately,
	// since one of them may be empty.

	// Start with the first submatrix.
	merged := responses[0]
	subRow := 0
	disRows, disIndex := 0, 0
	durRows, durIndex := 0, 0
	for _, res := range responses[1:] {
		if res.row != subRow {
			// On row changes, we simply append the rows of the current leftmost
			// submatrix to the merged matrix.
			subRow++
			disIndex += disRows
			durIndex += durRows
			merged.Distances = append(merged.Distances, res.Distances...)
			merged.Durations = append(merged.Durations, res.Durations...)
		} else {
			// On row stays, we append the columns of all rows individually.
			for i := 0; i < len(res.Distances); i++ {
				merged.Distances[disIndex+i] = append(
					merged.Distances[disIndex+i], res.Distances[i]...,
				)
			}
			for i := 0; i < len(res.Durations); i++ {
				merged.Durations[durIndex+i] = append(
					merged.Durations[durIndex+i], res.Durations[i]...,
				)
			}
			disRows = len(res.Distances)
			durRows = len(res.Durations)
		}
	}

	return merged
}

// deflateZeroes returns a new slice of points without points that are missing
// or [0,0]. It also returns a slice of indices that can be used to inflate the
// matrix. The indices are the indices of the points in the original slice.
func deflateZeroes(points []measure.Point) (deflated []measure.Point, indices []int) {
	deflated = make([]measure.Point, 0, len(points))
	indices = make([]int, 0, len(points))
	for i, point := range points {
		if len(point) == 0 || (point[0] == 0 && point[1] == 0) {
			continue
		}
		deflated = append(deflated, point)
		indices = append(indices, i)
	}
	return deflated, indices
}

// inflateZeroes returns a new matrix with inflated to original size. The
// indices are the indices of the points in the deflated matrix in the original
// matrix (i.e. the indices returned by deflateZeroes). The length is the length
// of the original matrix.
func inflateZeroes(matrix [][]float64, indices []int, length int) [][]float64 {
	inflated := make([][]float64, length)
	for i := 0; i < length; i++ {
		inflated[i] = make([]float64, length)
	}
	for i, oi := range indices {
		for j, oj := range indices {
			inflated[oi][oj] = matrix[i][j]
		}
	}
	return inflated
}
