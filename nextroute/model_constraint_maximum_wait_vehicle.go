package nextroute

import "github.com/nextmv-io/sdk/connect"

// MaximumWaitVehicleConstraint is a constraint that limits the accumulated time
// a vehicle can wait at stops on its route. Wait is defined as the
// time between starting a stop and arriving at the location of the stop,
// [SolutionStop.StartValue()] - [SolutionStop.ArrivalValue()].
type MaximumWaitVehicleConstraint interface {
	ModelConstraint

	// Maximum returns the maximum expression which defines the maximum
	// accumulated time a vehicle can wait on a route. Returns nil if not set.
	Maximum() VehicleTypeDurationExpression
}

// NewMaximumWaitVehicleConstraint returns a new MaximumWaitVehicleConstraint.
// The maximum wait constraint limits the accumulated time a vehicle can wait at
// stops on its route. Wait time is defined as the time between starting a stop
// and arriving at the location of the stop, [SolutionStop.StartValue()] -
// [SolutionStop.ArrivalValue()].
func NewMaximumWaitVehicleConstraint() (MaximumWaitVehicleConstraint, error) {
	connect.Connect(con, &newMaximumWaitVehicleConstraint)
	return newMaximumWaitVehicleConstraint()
}
