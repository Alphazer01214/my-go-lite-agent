package main

func lastFactSeq(facts []Fact) int {
	if len(facts) == 0 {
		return 0
	}
	return facts[len(facts)-1].Seq
}

func windowedTraceState(sessionID string, facts []Fact, revision int) traceStateUI {
	total := len(facts)
	start := total
	bytes := 0
	for start > 0 && total-start < traceWindowFacts {
		fact := facts[start-1]
		size := len(fact.Content) + len(fact.Meta) + len(fact.ToolCalls)
		if start < total && bytes+size > traceWindowBytes {
			break
		}
		bytes += size
		start--
	}
	window := facts[start:]
	state := traceStateUI{
		SessionID: sessionID, Revision: revision, ThroughSeq: lastFactSeq(facts),
		HasMore: start > 0, Total: total, Facts: window,
	}
	if len(window) > 0 {
		state.FirstSeq = window[0].Seq
	}
	return state
}
