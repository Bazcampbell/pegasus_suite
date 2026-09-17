// packages/core/vectorTypes.go

package core

import "math"

type Vector struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (v Vector) Add(o Vector) Vector {
	return Vector{
		Longitude: v.Longitude + o.Longitude,
		Latitude:  v.Latitude + o.Latitude,
	}
}

func (v Vector) Sub(o Vector) Vector {
	return Vector{
		Longitude: v.Longitude - o.Longitude,
		Latitude:  v.Latitude - o.Latitude,
	}
}

func (v Vector) Div(s float64) Vector {
	return Vector{
		Longitude: v.Longitude / s,
		Latitude:  v.Latitude / s,
	}
}

func (v Vector) Magnitude() float64 {
	return math.Sqrt(
		v.Longitude*v.Longitude +
			v.Latitude*v.Latitude,
	)
}

func (v Vector) Normalise() Vector {
	mag := v.Magnitude()

	if mag == 0 {
		return Vector{}
	}

	return Vector{
		Longitude: v.Longitude / mag,
		Latitude:  v.Latitude / mag,
	}
}

func (v Vector) Dot(o Vector) float64 {
	return v.Longitude*o.Longitude +
		v.Latitude*o.Latitude
}
