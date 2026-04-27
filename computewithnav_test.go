package timeseries

import (
	"errors"
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// Min / Max
// ---------------------------------------------------------------------------

func TestMin_skipsNaV(t *testing.T) {
	v, err := Min([]float64{3, NaV, 1, 2, NaV})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	assertFloat(t, v, 1, "Min skips NaV")
}

func TestMin_empty(t *testing.T) {
	v, err := Min(nil)
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("Min(nil) err = %v, want ErrEmptyInput", err)
	}
	if !IsNaV(v) {
		t.Errorf("Min(nil) value = %s, want NaV", Format(v))
	}
}

func TestMin_allNaV(t *testing.T) {
	v, err := Min([]float64{NaV, NaV, NaV})
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("err = %v, want ErrEmptyInput", err)
	}
	if !IsNaV(v) {
		t.Errorf("value = %s, want NaV", Format(v))
	}
}

func TestMax_skipsNaV(t *testing.T) {
	v, err := Max([]float64{3, NaV, 1, 2, NaV})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	assertFloat(t, v, 3, "Max skips NaV")
}

func TestMax_empty(t *testing.T) {
	v, err := Max(nil)
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("Max(nil) err = %v, want ErrEmptyInput", err)
	}
	if !IsNaV(v) {
		t.Errorf("value = %s, want NaV", Format(v))
	}
}

// ---------------------------------------------------------------------------
// StdDev: sample (n-1), NaV-skipping, ErrBounds when n < 2
// ---------------------------------------------------------------------------

func TestStdDev_sampleFormula(t *testing.T) {
	// {2, 4, 4, 4, 5, 5, 7, 9} — textbook example: sample std ≈ 2.138...
	// (population std = 2 exactly).
	v, err := StdDev([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := math.Sqrt(32.0 / 7.0) // sample variance = 32/7
	if math.Abs(v-want) > 1e-12 {
		t.Errorf("StdDev = %v, want %v", v, want)
	}
}

func TestStdDev_skipsNaV(t *testing.T) {
	base, _ := StdDev([]float64{1, 2, 3, 4, 5})
	withNaV, _ := StdDev([]float64{1, 2, NaV, 3, 4, 5})
	if math.Abs(base-withNaV) > 1e-12 {
		t.Errorf("StdDev skipsNaV: base=%v withNaV=%v", base, withNaV)
	}
}

func TestStdDev_empty(t *testing.T) {
	v, err := StdDev(nil)
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("err = %v, want ErrEmptyInput", err)
	}
	if !IsNaV(v) {
		t.Errorf("value = %s, want NaV", Format(v))
	}
}

func TestStdDev_singleValue_isUndefined(t *testing.T) {
	v, err := StdDev([]float64{42})
	if !errors.Is(err, ErrBounds) {
		t.Errorf("err = %v, want ErrBounds", err)
	}
	if !IsNaV(v) {
		t.Errorf("value = %s, want NaV", Format(v))
	}
}

func TestStdDev_onlyOneValidAmongNaVs(t *testing.T) {
	v, err := StdDev([]float64{NaV, 42, NaV})
	if !errors.Is(err, ErrBounds) {
		t.Errorf("err = %v, want ErrBounds", err)
	}
	if !IsNaV(v) {
		t.Errorf("value = %s, want NaV", Format(v))
	}
}

// ---------------------------------------------------------------------------
// Percentile
// ---------------------------------------------------------------------------

func TestPercentile_basic(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	// Nearest-rank (1-indexed) with k = floor(p/100*n).
	// p=50, n=10 → k=5 → data[4] = 5.
	v, err := Percentile(data, 50)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	assertFloat(t, v, 5, "50th percentile")
}

func TestPercentile_skipsNaV(t *testing.T) {
	data := []float64{1, NaV, 3, NaV, 5, 7, NaV}
	v, err := Percentile(data, 50)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// After skipping NaV: {1,3,5,7}, n=4, k = floor(0.5*4)=2, rank-1 = index 1 → 3.
	assertFloat(t, v, 3, "50th percentile skipping NaV")
}

func TestPercentile_outOfBounds(t *testing.T) {
	for _, p := range []float64{-1, 0, 101} {
		v, err := Percentile([]float64{1, 2, 3}, p)
		if !errors.Is(err, ErrBounds) {
			t.Errorf("p=%v: err=%v, want ErrBounds", p, err)
		}
		if !IsNaV(v) {
			t.Errorf("p=%v: v=%s, want NaV", p, Format(v))
		}
	}
}

func TestPercentile_doesNotModifyInput(t *testing.T) {
	data := []float64{3, 1, 4, 1, 5, 9, 2, 6}
	orig := append([]float64(nil), data...)
	_, _ = Percentile(data, 50)
	for i := range data {
		if data[i] != orig[i] {
			t.Fatalf("Percentile mutated input at %d: got %v, want %v", i, data[i], orig[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Median
// ---------------------------------------------------------------------------

func TestMedian_oddCount(t *testing.T) {
	v, err := Median([]float64{3, 1, 4, 1, 5})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	assertFloat(t, v, 3, "odd-count median")
}

func TestMedian_evenCount(t *testing.T) {
	v, err := Median([]float64{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	assertFloat(t, v, 2.5, "even-count median")
}

func TestMedian_skipsNaV(t *testing.T) {
	v, err := Median([]float64{1, NaV, 2, NaV, 3})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	assertFloat(t, v, 2, "median of {1,2,3}")
}

func TestMedian_emptyReturnsNaV(t *testing.T) {
	v, err := Median(nil)
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("err = %v, want ErrEmptyInput", err)
	}
	if !IsNaV(v) {
		t.Errorf("value = %s, want NaV", Format(v))
	}
}

func TestMedian_allNaV(t *testing.T) {
	v, err := Median([]float64{NaV, NaV})
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("err = %v, want ErrEmptyInput", err)
	}
	if !IsNaV(v) {
		t.Errorf("value = %s, want NaV", Format(v))
	}
}

func TestMedian_doesNotModifyInput(t *testing.T) {
	data := []float64{3, 1, 4, 1, 5, 9, 2, 6}
	orig := append([]float64(nil), data...)
	_, _ = Median(data)
	for i := range data {
		if data[i] != orig[i] {
			t.Fatalf("Median mutated input at %d: got %v, want %v", i, data[i], orig[i])
		}
	}
}
