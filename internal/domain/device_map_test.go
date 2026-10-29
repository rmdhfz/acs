package domain

import (
	"errors"
	"math"
	"testing"
)

func TestParseBBox(t *testing.T) {
	ok := map[string]BBox{
		"106.7,-6.4,107.0,-6.1":         {West: 106.7, South: -6.4, East: 107.0, North: -6.1},
		" 106.7 , -6.4 , 107.0 , -6.1 ": {West: 106.7, South: -6.4, East: 107.0, North: -6.1},
		"170,-10,-170,10":               {West: 170, South: -10, East: -170, North: 10}, // melintasi garis tanggal
		"-180,-90,180,90":               {West: -180, South: -90, East: 180, North: 90},
	}
	for raw, want := range ok {
		got, err := ParseBBox(raw)
		if err != nil || *got != want {
			t.Errorf("ParseBBox(%q) = %+v, %v; mau %+v", raw, got, err, want)
		}
	}
	bad := []string{
		"", "1,2,3", "1,2,3,4,5", "a,b,c,d", "1,2,3,", "NaN,0,1,1", "0,0,Inf,1",
		"0,10,1,-10",  // selatan > utara
		"0,-91,1,0",   // lintang di luar
		"0,0,1,91",    // lintang di luar
		"-181,0,1,1",  // bujur di luar
		"0,0,181,1",   // bujur di luar
		"1e999,0,1,1", // luap
	}
	for _, raw := range bad {
		if b, err := ParseBBox(raw); err == nil || !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ParseBBox(%q) = %+v, %v; mau ErrInvalidInput", raw, b, err)
		}
	}
}

func TestValidCoordinate(t *testing.T) {
	for _, c := range [][2]float64{{-6.2, 106.8}, {0, 0}, {90, 180}, {-90, -180}} {
		if !ValidCoordinate(c[0], c[1]) {
			t.Errorf("%v seharusnya valid", c)
		}
	}
	for _, c := range [][2]float64{{91, 0}, {0, 181}, {-91, 0}, {0, -181}, {math.NaN(), 0}, {0, math.Inf(1)}} {
		if ValidCoordinate(c[0], c[1]) {
			t.Errorf("%v seharusnya tidak valid", c)
		}
	}
}
