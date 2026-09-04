package market

// Regime labels a market state for persistence/transition purposes (regime
// history tracking, transition-probability math). It intentionally has the
// same values as State — see PR-073: the two independently-evolved
// classification pipelines (softmax scores vs. proportional breadth) were
// unified onto Summary/State/Breadth, but Regime survives as the shared
// vocabulary for regimehistory and transition, which only need a labeled
// state, not a full classification result.
type Regime string

const (
	RegimeCompression Regime = "compression"
	RegimeSideways    Regime = "sideways"
	RegimeTrend       Regime = "trend"
	RegimeExpansion   Regime = "expansion"
	RegimeIndecisive  Regime = "indecisive"
	RegimeSilent      Regime = "silent"
)
