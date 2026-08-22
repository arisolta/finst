package engine

import (
	"math"
	"sort"
	"time"

	"finst/internal/model"
)

type Forecaster struct{}

func NewForecaster() *Forecaster {
	return &Forecaster{}
}

// ProjectionRatios holds median 3-year historical ratios.
type ProjectionRatios struct {
	CAGR          float64
	MedianGrossM  float64
	MedianEBITDAM float64
	MedianNetM    float64
	MedianCapExR  float64
	MedianDAR     float64
}

// Compute3YearRatios calculates the median historical margins and clamped CAGR.
func (f *Forecaster) Compute3YearRatios(historical []model.FinancialStatement) ProjectionRatios {
	var grossMargins []float64
	var ebitdaMargins []float64
	var netMargins []float64
	var capexRatios []float64
	var daRatios []float64

	for _, st := range historical {
		if st.Revenue > 0 {
			gm := st.GrossProfit / st.Revenue
			grossMargins = append(grossMargins, gm)

			ebitda := st.OperatingIncome + st.DepreciationAmortization
			ebitdaMargins = append(ebitdaMargins, ebitda/st.Revenue)

			nm := st.NetIncome / st.Revenue
			netMargins = append(netMargins, nm)

			capex := math.Abs(st.CapEx)
			capexRatios = append(capexRatios, capex/st.Revenue)

			da := st.DepreciationAmortization
			daRatios = append(daRatios, da/st.Revenue)
		}
	}

	cagr := 0.05 // default 5%
	if len(historical) >= 3 {
		first := historical[0].Revenue
		last := historical[len(historical)-1].Revenue
		cagr = ComputeClampedCAGR(first, last, float64(len(historical)-1))
	} else if len(historical) >= 2 {
		first := historical[0].Revenue
		last := historical[len(historical)-1].Revenue
		cagr = ComputeClampedCAGR(first, last, 1.0)
	}

	return ProjectionRatios{
		CAGR:          cagr,
		MedianGrossM:  median(grossMargins, 0.50), // fallback 50%
		MedianEBITDAM: median(ebitdaMargins, 0.20),
		MedianNetM:    median(netMargins, 0.15),
		MedianCapExR:  median(capexRatios, 0.05),
		MedianDAR:     median(daRatios, 0.05),
	}
}

// ProjectForwardYear generates estimates for a given year using consensus or heuristic model.
func (f *Forecaster) ProjectForwardYear(
	targetFY int,
	prevRevenue float64,
	dilutedShares float64,
	ratios ProjectionRatios,
	consensus *model.ConsensusEstimate,
) model.PeriodData {
	isConsensus := (consensus != nil && (consensus.EstRevenue > 0 || consensus.EstEPS != 0))

	var rev, grossProfit, ebitda, netIncome, eps, capex, da, cfo, fcf float64
	sourceLabel := "Proj"
	periodType := model.PeriodTypeProjection

	if isConsensus {
		sourceLabel = "Cons"
		periodType = model.PeriodTypeConsensus
		if consensus.EstRevenue > 0 {
			rev = consensus.EstRevenue
		} else {
			rev = prevRevenue * (1.0 + ratios.CAGR)
		}

		if consensus.EstEPS != 0 {
			eps = consensus.EstEPS
			if dilutedShares > 0 {
				netIncome = eps * dilutedShares
			} else {
				netIncome = rev * ratios.MedianNetM
			}
		} else if consensus.EstNetIncome != 0 {
			netIncome = consensus.EstNetIncome
			if dilutedShares > 0 {
				eps = netIncome / dilutedShares
			}
		} else {
			netIncome = rev * ratios.MedianNetM
			if dilutedShares > 0 {
				eps = netIncome / dilutedShares
			}
		}

		if consensus.EstEBITDA > 0 {
			ebitda = consensus.EstEBITDA
		} else {
			ebitda = rev * ratios.MedianEBITDAM
		}

		grossProfit = rev * ratios.MedianGrossM
		capex = -(rev * ratios.MedianCapExR)
		da = rev * ratios.MedianDAR

		if consensus.EstCFO != 0 {
			cfo = consensus.EstCFO
		} else {
			// CFO = Net Income + D&A - Delta NWC (2% of incremental Revenue)
			deltaRev := rev - prevRevenue
			if deltaRev < 0 {
				deltaRev = 0
			}
			deltaNWC := 0.02 * deltaRev
			cfo = netIncome + da - deltaNWC
		}

		if consensus.EstCapEx != 0 {
			capex = -math.Abs(consensus.EstCapEx)
		}
		fcf = cfo - math.Abs(capex)
	} else {
		// Heuristic Fallback Engine
		rev = prevRevenue * (1.0 + ratios.CAGR)
		grossProfit = rev * ratios.MedianGrossM
		ebitda = rev * ratios.MedianEBITDAM
		netIncome = rev * ratios.MedianNetM
		if dilutedShares > 0 {
			eps = netIncome / dilutedShares
		}
		capex = -(rev * ratios.MedianCapExR)
		da = rev * ratios.MedianDAR

		deltaRev := rev - prevRevenue
		if deltaRev < 0 {
			deltaRev = 0
		}
		deltaNWC := 0.02 * deltaRev
		cfo = netIncome + da - deltaNWC
		fcf = cfo - math.Abs(capex)
	}

	gmPct := (grossProfit / rev) * 100
	ebitdaPct := (ebitda / rev) * 100
	ebit := ebitda - da
	ebitPct := (ebit / rev) * 100
	netPct := (netIncome / rev) * 100
	var fcfConvPct *float64
	if netIncome > 0 {
		conv := (fcf / netIncome) * 100
		fcfConvPct = &conv
	}

	return model.PeriodData{
		Label:                    formatPeriodLabel(targetFY, sourceLabel),
		FiscalYear:               targetFY,
		PeriodType:               periodType,
		IsForward:                true,
		Revenue:                  rev,
		GrossProfit:              grossProfit,
		GrossMarginPct:           &gmPct,
		EBITDA:                   ebitda,
		EBITDAMarginPct:          &ebitdaPct,
		EBIT:                     ebit,
		EBITMarginPct:            &ebitPct,
		NetIncome:                netIncome,
		NetMarginPct:             &netPct,
		DilutedAdjEPS:            eps,
		OperatingCashFlow:        cfo,
		DepreciationAmortization: da,
		CapEx:                    capex,
		FreeCashFlow:             fcf,
		FCFConversionPct:         fcfConvPct,
	}
}

func median(values []float64, fallback float64) float64 {
	if len(values) == 0 {
		return fallback
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2.0
}

func formatPeriodLabel(year int, source string) string {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006") + "E (" + source + ")"
}
