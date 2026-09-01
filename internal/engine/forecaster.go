package engine

import (
	"math"
	"time"

	"github.com/arisolta/finst/internal/model"
)

type Forecaster struct{}

func NewForecaster() *Forecaster {
	return &Forecaster{}
}

// ProjectionRatios holds time-weighted historical ratios.
type ProjectionRatios struct {
	CAGR            float64
	WeightedGrossM  float64
	WeightedEBITDAM float64
	WeightedNetM    float64
	WeightedCapExR  float64
	WeightedDAR     float64
}

// Compute3YearRatios calculates the time-weighted historical margins and clamped CAGR.
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
		CAGR:            cagr,
		WeightedGrossM:  timeWeightedRatio(grossMargins, 0.50),
		WeightedEBITDAM: timeWeightedRatio(ebitdaMargins, 0.20),
		WeightedNetM:    timeWeightedRatio(netMargins, 0.15),
		WeightedCapExR:  timeWeightedRatio(capexRatios, 0.05),
		WeightedDAR:     timeWeightedRatio(daRatios, 0.05),
	}
}

// ProjectForwardYear generates estimates for a given year using consensus or Option 2 blended model.
func (f *Forecaster) ProjectForwardYear(
	targetFY int,
	prevRevenue float64,
	dilutedShares float64,
	ratios ProjectionRatios,
	consensus *model.ConsensusEstimate,
	priorForwardPeriods []model.PeriodData,
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
				if rev > 0 && ratios.WeightedNetM > 0 {
					impliedMargin := netIncome / rev
					if impliedMargin < 0.40*ratios.WeightedNetM || impliedMargin > 2.50*ratios.WeightedNetM {
						netIncome = rev * ratios.WeightedNetM
						eps = netIncome / dilutedShares
					}
				}
			} else {
				netIncome = rev * ratios.WeightedNetM
			}
		} else if consensus.EstNetIncome != 0 {
			netIncome = consensus.EstNetIncome
			if dilutedShares > 0 {
				eps = netIncome / dilutedShares
			}
		} else {
			netIncome = rev * ratios.WeightedNetM
			if dilutedShares > 0 {
				eps = netIncome / dilutedShares
			}
		}

		if consensus.EstEBITDA > 0 {
			ebitda = consensus.EstEBITDA
		} else {
			ebitda = rev * ratios.WeightedEBITDAM
		}

		grossProfit = rev * ratios.WeightedGrossM
		capex = -(rev * ratios.WeightedCapExR)
		da = rev * ratios.WeightedDAR

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
		// Option 2: Blended Growth & Margin Smoothing
		// If we have built prior forward periods (e.g. T+1 and T+2), blend forward momentum with historical baseline
		nPrior := len(priorForwardPeriods)
		if nPrior >= 2 && priorForwardPeriods[nPrior-1].Revenue > 0 && priorForwardPeriods[nPrior-2].Revenue > 0 {
			p2 := priorForwardPeriods[nPrior-1] // T+2 (e.g. 2027E)
			p1 := priorForwardPeriods[nPrior-2] // T+1 (e.g. 2026E)

			// 1. Blended Revenue Growth: 65% consensus momentum + 35% historical CAGR
			gCons := (p2.Revenue - p1.Revenue) / p1.Revenue
			gBlended := 0.65*gCons + 0.35*ratios.CAGR
			gBlended = math.Max(-0.05, math.Min(0.25, gBlended))
			rev = p2.Revenue * (1.0 + gBlended)

			// 2. Exponentially smoothed margins: 60% T+2, 25% T+1, 15% structural baseline
			gm2 := p2.GrossProfit / p2.Revenue
			gm1 := p1.GrossProfit / p1.Revenue
			gmBlended := 0.60*gm2 + 0.25*gm1 + 0.15*ratios.WeightedGrossM
			grossProfit = rev * gmBlended

			em2 := p2.EBITDA / p2.Revenue
			em1 := p1.EBITDA / p1.Revenue
			emBlended := 0.60*em2 + 0.25*em1 + 0.15*ratios.WeightedEBITDAM
			ebitda = rev * emBlended

			nm2 := p2.NetIncome / p2.Revenue
			nm1 := p1.NetIncome / p1.Revenue
			nmBlended := 0.60*nm2 + 0.25*nm1 + 0.15*ratios.WeightedNetM
			netIncome = rev * nmBlended
			if dilutedShares > 0 {
				eps = netIncome / dilutedShares
			}

			da2 := p2.DepreciationAmortization / p2.Revenue
			da1 := p1.DepreciationAmortization / p1.Revenue
			daBlended := 0.60*da2 + 0.25*da1 + 0.15*ratios.WeightedDAR
			da = rev * daBlended

			capex2 := math.Abs(p2.CapEx) / p2.Revenue
			capex1 := math.Abs(p1.CapEx) / p1.Revenue
			capexBlended := 0.60*capex2 + 0.25*capex1 + 0.15*ratios.WeightedCapExR
			capex = -(rev * capexBlended)

			deltaRev := rev - p2.Revenue
			if deltaRev < 0 {
				deltaRev = 0
			}
			deltaNWC := 0.02 * deltaRev
			cfo = netIncome + da - deltaNWC
			fcf = cfo - math.Abs(capex)
		} else {
			// Single-period projection from prior historical
			rev = prevRevenue * (1.0 + ratios.CAGR)
			grossProfit = rev * ratios.WeightedGrossM
			ebitda = rev * ratios.WeightedEBITDAM
			netIncome = rev * ratios.WeightedNetM
			if dilutedShares > 0 {
				eps = netIncome / dilutedShares
			}
			capex = -(rev * ratios.WeightedCapExR)
			da = rev * ratios.WeightedDAR

			deltaRev := rev - prevRevenue
			if deltaRev < 0 {
				deltaRev = 0
			}
			deltaNWC := 0.02 * deltaRev
			cfo = netIncome + da - deltaNWC
			fcf = cfo - math.Abs(capex)
		}
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

func timeWeightedRatio(values []float64, fallback float64) float64 {
	if len(values) == 0 {
		return fallback
	}
	if len(values) == 1 {
		return values[0]
	}
	if len(values) == 2 {
		return 0.65*values[1] + 0.35*values[0]
	}
	n := len(values)
	return 0.60*values[n-1] + 0.25*values[n-2] + 0.15*values[n-3]
}

func formatPeriodLabel(year int, source string) string {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006") + "E (" + source + ")"
}
