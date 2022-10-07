// © 2019-2022 nextmv.io inc. All rights reserved.
// nextmv.io, inc. CONFIDENTIAL
//
// This file includes unpublished proprietary source code of nextmv.io, inc.
// The copyright notice above does not evidence any actual or intended
// publication of such source code. Disclosure of this source code or any
// related proprietary information is strictly prohibited without the express
// written permission of nextmv.io, inc.

/*
Package google provides functions for measuring distances and durations using
the Google Distance Matrix API and polylines from Google Maps Distance API. A
Google Maps client and request are required. The client uses an API key for
authentication.
Matrix API: At a minimum, the request requires the origins and destinations
to estimate.
Distance API: At minimum, the request requires the origin and destination. But
it is recommended to pass in waypoints as an encoded as a polyline with "enc:"
as a prefix to get a more precise polyline for each leg of the route.

Here is a minimal example of how to create a client and matrix request,
assuming the points are in the form longitude, latitude:

	points := [][2]float64{
	    {-74.028297, 4.875835},
	    {-74.046965, 4.872842},
	    {-74.041763, 4.885648},
	}
	coords := make([]string, len(points))
	for p, point := range points {
	    coords[p] = fmt.Sprintf("%f,%f", point[1], point[0])
	}
	r := &maps.DistanceMatrixRequest{
	    Origins:      coords,
	    Destinations: coords,
	}
	c, err := maps.NewClient(maps.WithAPIKey("<your-api-key>"))
	if err != nil {
	    panic(err)
	}

Distance and duration matrices can be constructed with the functions provided in
the package.

	dist, dur, err := google.DistanceDurationMatrices(c, r)
	if err != nil {
	    panic(err)
	}

Once the measures have been created, you may estimate the distances and
durations by calling the Cost function:

	for p1 := range points {
	    for p2 := range points {
	        fmt.Printf(
	            "(%d, %d) = [%f, %f]\n",
	            p1, p2, dist.Cost(p1, p2), dur.Cost(p1, p2),
	        )
	    }
	}

This should print the following result, which is in the format (from, to) =
[distance, duration]:

	(0, 0) = [0.000000, 0.000000]
	(0, 1) = [6526.000000, 899.000000]
	(0, 2) = [4889.000000, 669.000000]
	(1, 0) = [5211.000000, 861.000000]
	(1, 1) = [0.000000, 0.000000]
	(1, 2) = [2260.000000, 302.000000]
	(2, 0) = [3799.000000, 638.000000]
	(2, 1) = [2260.000000, 311.000000]
	(2, 2) = [0.000000, 0.000000]

Making a request to retrieve polylines works similar. In this example we reuse
the same points and client from above and create a DirectionsRequest. The
polylines function returns a polyline from start to end and a slice of polylines
for each leg, given through the waypoints. All polylines are encoded in Google's
polyline format.

	rPoly := &maps.DirectionsRequest{
		Origin:      coords[0],
		Destination: coords[len(coords)-1],
		Waypoints:   coords[1 : len(coords)-1],
	}

	fullPoly, polyLegs, err := google.Polylines(c, rPoly)
	if err != nil {
		panic(err)
	}
*/
package google
