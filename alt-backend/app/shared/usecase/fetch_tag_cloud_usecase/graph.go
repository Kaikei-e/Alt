package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"cmp"
	"math"
	"slices"
)

const (
	maxPartnersPerTag    = 3
	maxKeptEdges         = 600
	minCommunityTagCount = 4
	maxCommunities       = 8

	// Resolution parameter γ > 1 for the Reichardt-Bornholdt configuration null
	// model discounts high-strength hub tags and prevents cross-topic merges
	// while keeping sparse intra-topic clusters cohesive.
	leidenResolution = 1.25
	maxLeidenLevels  = 10
	maxMovePasses    = 30
	qualityEps       = 1e-12
)

type indexPair struct {
	u, v int
}

type partnerEdge struct {
	partner int
	ochiai  float64
}

type rawEdge struct {
	sourceIndex int
	targetIndex int
	ochiai      float64
}

// buildAssociationGraph ranks tags in the fixed order (article count
// descending, then tag name ascending) and returns fixedOrder (rank -> item
// index) together with the undirected Ochiai adjacency list indexed by rank.
func buildAssociationGraph(items []*domain.TagCloudItem, cooccurrences []*domain.TagCooccurrence) ([]int, [][]partnerEdge) {
	n := len(items)
	fixedOrder := make([]int, n)
	for i := range n {
		fixedOrder[i] = i
	}
	slices.SortFunc(fixedOrder, func(i, j int) int {
		if c := cmp.Compare(items[j].ArticleCount, items[i].ArticleCount); c != 0 {
			return c
		}
		return cmp.Compare(items[i].TagName, items[j].TagName)
	})

	nameToRank := make(map[string]int, n)
	for rank, itemIdx := range fixedOrder {
		nameToRank[items[itemIdx].TagName] = rank
	}

	pairOchiai := make(map[indexPair]float64, len(cooccurrences))
	for _, c := range cooccurrences {
		if c.SharedCount <= 0 {
			continue
		}
		rankA, okA := nameToRank[c.TagNameA]
		rankB, okB := nameToRank[c.TagNameB]
		if !okA || !okB || rankA == rankB {
			continue
		}
		countA := items[fixedOrder[rankA]].ArticleCount
		countB := items[fixedOrder[rankB]].ArticleCount
		if countA <= 0 || countB <= 0 {
			continue
		}
		ochiai := float64(c.SharedCount) / math.Sqrt(float64(countA)*float64(countB))
		u, v := rankA, rankB
		if u > v {
			u, v = v, u
		}
		key := indexPair{u: u, v: v}
		if prev, exists := pairOchiai[key]; !exists || ochiai > prev {
			pairOchiai[key] = ochiai
		}
	}

	if len(pairOchiai) == 0 {
		return fixedOrder, nil
	}

	adj := make([][]partnerEdge, n)
	for pair, w := range pairOchiai {
		adj[pair.u] = append(adj[pair.u], partnerEdge{partner: pair.v, ochiai: w})
		adj[pair.v] = append(adj[pair.v], partnerEdge{partner: pair.u, ochiai: w})
	}
	// Sort neighbour lists by fixed-order rank so floating-point summation and
	// tie-breaking are independent of map iteration or input slice order.
	for r := range n {
		slices.SortFunc(adj[r], func(a, b partnerEdge) int {
			return cmp.Compare(a.partner, b.partner)
		})
	}

	return fixedOrder, adj
}

// ComputeEdges selects each tag's 3 strongest partners by Ochiai coefficient,
// takes the deduplicated undirected union (source_index < target_index), caps
// at the 600 strongest, normalises weights to (0, 1] by the maximum kept
// Ochiai, and sorts by weight descending, then source_index, then target_index.
func ComputeEdges(items []*domain.TagCloudItem, cooccurrences []*domain.TagCooccurrence) []*domain.TagCloudEdge {
	if len(items) < 2 || len(cooccurrences) == 0 {
		return nil
	}

	fixedOrder, adj := buildAssociationGraph(items, cooccurrences)
	if adj == nil {
		return nil
	}

	keptMap := make(map[indexPair]float64)
	for r, partners := range adj {
		if len(partners) == 0 {
			continue
		}
		sorted := slices.Clone(partners)
		// Break equal-Ochiai partner ties by fixed rank (article count desc,
		// tag name asc) so kept partners do not depend on input slice order.
		slices.SortFunc(sorted, func(a, b partnerEdge) int {
			if c := cmp.Compare(b.ochiai, a.ochiai); c != 0 {
				return c
			}
			return cmp.Compare(a.partner, b.partner)
		})
		limit := min(len(sorted), maxPartnersPerTag)
		itemIdx := fixedOrder[r]
		for _, p := range sorted[:limit] {
			u, v := itemIdx, fixedOrder[p.partner]
			if u > v {
				u, v = v, u
			}
			keptMap[indexPair{u: u, v: v}] = p.ochiai
		}
	}

	kept := make([]rawEdge, 0, len(keptMap))
	for pair, w := range keptMap {
		kept = append(kept, rawEdge{
			sourceIndex: pair.u,
			targetIndex: pair.v,
			ochiai:      w,
		})
	}

	slices.SortFunc(kept, func(a, b rawEdge) int {
		if c := cmp.Compare(b.ochiai, a.ochiai); c != 0 {
			return c
		}
		if c := cmp.Compare(a.sourceIndex, b.sourceIndex); c != 0 {
			return c
		}
		return cmp.Compare(a.targetIndex, b.targetIndex)
	})

	if len(kept) > maxKeptEdges {
		kept = kept[:maxKeptEdges]
	}

	maxWeight := kept[0].ochiai
	result := make([]*domain.TagCloudEdge, len(kept))
	for i, e := range kept {
		result[i] = &domain.TagCloudEdge{
			SourceIndex: e.sourceIndex,
			TargetIndex: e.targetIndex,
			Weight:      e.ochiai / maxWeight,
		}
	}

	slices.SortFunc(result, func(a, b *domain.TagCloudEdge) int {
		if c := cmp.Compare(b.Weight, a.Weight); c != 0 {
			return c
		}
		if c := cmp.Compare(a.SourceIndex, b.SourceIndex); c != 0 {
			return c
		}
		return cmp.Compare(a.TargetIndex, b.TargetIndex)
	})

	return result
}

type leidenGraph struct {
	n        int
	adj      [][]partnerEdge // undirected neighbours (no self-loops), sorted by partner asc
	strength []float64       // total node strength (invariant across aggregation levels)
	seedRank []int           // minimum original fixedOrder rank represented by super-node
	twoM     float64         // sum of all node strengths (invariant across aggregation levels)
}

// ComputeClusters assigns topic communities via the deterministic Leiden
// algorithm optimising Reichardt-Bornholdt configuration modularity over the
// full Ochiai association graph. Communities with fewer than 4 tags receive
// cluster 0, and up to the 8 largest communities by total article count
// receive clusters 1..8.
func ComputeClusters(items []*domain.TagCloudItem, cooccurrences []*domain.TagCooccurrence) {
	n := len(items)
	for _, item := range items {
		item.ClusterID = 0
	}
	if n < minCommunityTagCount || len(cooccurrences) == 0 {
		return
	}

	fixedOrder, adj := buildAssociationGraph(items, cooccurrences)
	if adj == nil {
		return
	}

	strength := make([]float64, n)
	seedRank := make([]int, n)
	twoM := 0.0
	for r := range n {
		seedRank[r] = r
		s := 0.0
		for _, nb := range adj[r] {
			s += nb.ochiai
		}
		strength[r] = s
		twoM += s
	}
	if twoM <= 0 {
		return
	}

	labels := runLeiden(&leidenGraph{
		n:        n,
		adj:      adj,
		strength: strength,
		seedRank: seedRank,
		twoM:     twoM,
	}, leidenResolution)

	type community struct {
		seedRank     int
		memberRanks  []int
		totalArticle int
	}

	byLabel := make(map[int]*community)
	for r, lbl := range labels {
		comm, ok := byLabel[lbl]
		if !ok {
			comm = &community{seedRank: r}
			byLabel[lbl] = comm
		} else if r < comm.seedRank {
			comm.seedRank = r
		}
		comm.memberRanks = append(comm.memberRanks, r)
		comm.totalArticle += items[fixedOrder[r]].ArticleCount
	}

	qualifying := make([]*community, 0, len(byLabel))
	for _, comm := range byLabel {
		if len(comm.memberRanks) >= minCommunityTagCount {
			qualifying = append(qualifying, comm)
		}
	}

	slices.SortFunc(qualifying, func(a, b *community) int {
		if c := cmp.Compare(b.totalArticle, a.totalArticle); c != 0 {
			return c
		}
		return cmp.Compare(a.seedRank, b.seedRank)
	})

	if len(qualifying) > maxCommunities {
		qualifying = qualifying[:maxCommunities]
	}

	for clusterIdx, comm := range qualifying {
		clusterID := clusterIdx + 1
		for _, r := range comm.memberRanks {
			items[fixedOrder[r]].ClusterID = clusterID
		}
	}
}

func runLeiden(g0 *leidenGraph, gamma float64) []int {
	g := g0
	comm := make([]int, g.n)
	origToNode := make([]int, g0.n)
	for i := range g.n {
		comm[i] = i
		origToNode[i] = i
	}

	for range maxLeidenLevels {
		moved := leidenMoveNodes(g, comm, gamma)
		parentComm, numParent := canonicalisePartition(g, comm)

		if !moved && numParent == g.n {
			comm = parentComm
			break
		}

		refComm, numRefined := leidenRefine(g, parentComm, numParent, gamma)
		if numRefined == g.n {
			comm = parentComm
			break
		}

		gNext, nextInitComm, nodeToNext := leidenAggregate(g, parentComm, refComm, numRefined)
		for r, cur := range origToNode {
			origToNode[r] = nodeToNext[cur]
		}
		g = gNext
		comm = nextInitComm
	}

	finalComm, _ := canonicalisePartition(g, comm)
	result := make([]int, g0.n)
	for r, superIdx := range origToNode {
		result[r] = finalComm[superIdx]
	}
	return result
}

// leidenMoveNodes runs Leiden's fast queue-based local moving phase in
// canonical node order, returning true if at least one node changed community.
func leidenMoveNodes(g *leidenGraph, comm []int, gamma float64) bool {
	m := g.n
	sigmaTot := make([]float64, m)
	commSize := make([]int, m)
	for u := range m {
		c := comm[u]
		sigmaTot[c] += g.strength[u]
		commSize[c]++
	}

	freeComms := make([]int, 0, m)
	for c := m - 1; c >= 0; c-- {
		if commSize[c] == 0 {
			freeComms = append(freeComms, c)
		}
	}

	minSeedOfComm := func(target int) int {
		best := math.MaxInt
		for v := range m {
			if comm[v] == target && g.seedRank[v] < best {
				best = g.seedRank[v]
			}
		}
		return best
	}

	queue := make([]int, m, m*(maxMovePasses+1))
	inQueue := make([]bool, m)
	for u := range m {
		queue[u] = u
		inQueue[u] = true
	}

	commWeights := make([]float64, m)
	commSeen := make([]bool, m)
	touched := make([]int, 0, 16)

	scale := gamma / g.twoM
	anyMoved := false
	maxSteps := m * maxMovePasses

	for head := 0; head < len(queue) && head < maxSteps; head++ {
		u := queue[head]
		inQueue[u] = false

		oldComm := comm[u]
		if len(g.adj[u]) == 0 && commSize[oldComm] == 1 {
			continue
		}

		touched = touched[:0]
		for _, nb := range g.adj[u] {
			c := comm[nb.partner]
			if !commSeen[c] {
				commSeen[c] = true
				touched = append(touched, c)
			}
			commWeights[c] += nb.ochiai
		}

		ku := g.strength[u]
		wOld := commWeights[oldComm]
		stayScore := wOld - scale*ku*(sigmaTot[oldComm]-ku)

		bestComm := oldComm
		bestDelta := 0.0
		bestSeed := -1

		// Consider isolating u into its own singleton community when oldComm has >1 members.
		isolateSelected := false
		if commSize[oldComm] > 1 {
			isolateDelta := -stayScore
			if isolateDelta > qualityEps {
				bestDelta = isolateDelta
				bestSeed = g.seedRank[u]
				isolateSelected = true
			}
		}

		slices.Sort(touched)
		for _, c := range touched {
			if c == oldComm {
				continue
			}
			candScore := commWeights[c] - scale*ku*sigmaTot[c]
			delta := candScore - stayScore
			if delta <= qualityEps {
				continue
			}
			if delta > bestDelta+1e-15 {
				bestDelta = delta
				bestComm = c
				bestSeed = -1
				isolateSelected = false
			} else if math.Abs(delta-bestDelta) <= 1e-15 {
				if bestSeed < 0 {
					bestSeed = minSeedOfComm(bestComm)
				}
				candSeed := minSeedOfComm(c)
				if candSeed < bestSeed {
					bestDelta = delta
					bestComm = c
					bestSeed = candSeed
					isolateSelected = false
				}
			}
		}

		for _, c := range touched {
			commWeights[c] = 0
			commSeen[c] = false
		}

		if isolateSelected {
			bestComm = freeComms[len(freeComms)-1]
			freeComms = freeComms[:len(freeComms)-1]
		} else if bestComm == oldComm {
			continue
		}

		anyMoved = true
		comm[u] = bestComm
		sigmaTot[oldComm] -= ku
		commSize[oldComm]--
		if commSize[oldComm] == 0 {
			freeComms = append(freeComms, oldComm)
		}
		sigmaTot[bestComm] += ku
		commSize[bestComm]++

		for _, nb := range g.adj[u] {
			v := nb.partner
			if comm[v] != bestComm && !inQueue[v] {
				queue = append(queue, v)
				inQueue[v] = true
			}
		}
	}

	return anyMoved
}

// canonicalisePartition remaps community IDs to 0..k-1 ordered by each
// community's minimum seedRank.
func canonicalisePartition(g *leidenGraph, comm []int) ([]int, int) {
	minSeed := make(map[int]int)
	for u, c := range comm {
		if prev, ok := minSeed[c]; !ok || g.seedRank[u] < prev {
			minSeed[c] = g.seedRank[u]
		}
	}

	type commInfo struct {
		oldID int
		seed  int
	}
	infos := make([]commInfo, 0, len(minSeed))
	for oldID, seed := range minSeed {
		infos = append(infos, commInfo{oldID: oldID, seed: seed})
	}
	slices.SortFunc(infos, func(a, b commInfo) int {
		return cmp.Compare(a.seed, b.seed)
	})

	remap := make(map[int]int, len(infos))
	for newID, info := range infos {
		remap[info.oldID] = newID
	}

	out := make([]int, g.n)
	for u, c := range comm {
		out[u] = remap[c]
	}
	return out, len(infos)
}

// leidenRefine splits or regroups each parent community from singletons so
// every aggregated super-node is guaranteed to be internally well-connected.
func leidenRefine(g *leidenGraph, parentComm []int, numParent int, gamma float64) ([]int, int) {
	m := g.n
	parentSigmaTot := make([]float64, numParent)
	parentMembers := make([][]int, numParent)
	for u := range m {
		p := parentComm[u]
		parentSigmaTot[p] += g.strength[u]
		parentMembers[p] = append(parentMembers[p], u)
	}

	refComm := make([]int, m)
	refSize := make([]int, m)
	refSigmaTot := make([]float64, m)
	refCutToParent := make([]float64, m)
	nodeWeightInParent := make([]float64, m)

	for u := range m {
		refComm[u] = u
		refSize[u] = 1
		refSigmaTot[u] = g.strength[u]
		p := parentComm[u]
		wInP := 0.0
		for _, nb := range g.adj[u] {
			if parentComm[nb.partner] == p {
				wInP += nb.ochiai
			}
		}
		nodeWeightInParent[u] = wInP
		refCutToParent[u] = wInP
	}

	refWeights := make([]float64, m)
	refSeen := make([]bool, m)
	touched := make([]int, 0, 16)
	scale := gamma / g.twoM

	for p := range numParent {
		members := parentMembers[p]
		if len(members) <= 1 {
			continue
		}
		pTot := parentSigmaTot[p]

		for _, u := range members {
			if refSize[refComm[u]] != 1 {
				continue
			}
			ku := g.strength[u]
			// Node u must be well-connected to P \ {u}.
			if nodeWeightInParent[u]+1e-15 < scale*ku*(pTot-ku) {
				continue
			}

			touched = touched[:0]
			for _, nb := range g.adj[u] {
				v := nb.partner
				if parentComm[v] != p {
					continue
				}
				r := refComm[v]
				if !refSeen[r] {
					refSeen[r] = true
					touched = append(touched, r)
				}
				refWeights[r] += nb.ochiai
			}

			bestRef := -1
			bestDelta := qualityEps
			slices.Sort(touched)
			for _, r := range touched {
				if r == refComm[u] {
					continue
				}
				rTot := refSigmaTot[r]
				// Candidate refined sub-community R must also be well-connected to P \ R.
				if refCutToParent[r]+1e-15 < scale*rTot*(pTot-rTot) {
					continue
				}
				delta := refWeights[r] - scale*ku*rTot
				if delta > bestDelta+1e-15 {
					bestDelta = delta
					bestRef = r
				}
			}

			if bestRef >= 0 {
				wUR := refWeights[bestRef]
				oldRef := refComm[u]
				refComm[u] = bestRef
				refSize[oldRef] = 0
				refSigmaTot[oldRef] = 0
				refCutToParent[oldRef] = 0

				refSize[bestRef]++
				refSigmaTot[bestRef] += ku
				refCutToParent[bestRef] = refCutToParent[bestRef] + nodeWeightInParent[u] - 2*wUR
			}

			for _, r := range touched {
				refWeights[r] = 0
				refSeen[r] = false
			}
		}
	}

	return canonicalisePartition(g, refComm)
}

func leidenAggregate(g *leidenGraph, parentComm []int, refComm []int, numRefined int) (*leidenGraph, []int, []int) {
	strength := make([]float64, numRefined)
	seedRank := make([]int, numRefined)
	for i := range numRefined {
		seedRank[i] = math.MaxInt
	}
	initComm := make([]int, numRefined)

	for u := range g.n {
		r := refComm[u]
		strength[r] += g.strength[u]
		if g.seedRank[u] < seedRank[r] {
			seedRank[r] = g.seedRank[u]
		}
		initComm[r] = parentComm[u]
	}

	pairWeights := make(map[indexPair]float64)
	for u := range g.n {
		rU := refComm[u]
		for _, nb := range g.adj[u] {
			if nb.partner <= u {
				continue
			}
			rV := refComm[nb.partner]
			if rU == rV {
				continue
			}
			a, b := rU, rV
			if a > b {
				a, b = b, a
			}
			pairWeights[indexPair{u: a, v: b}] += nb.ochiai
		}
	}

	adj := make([][]partnerEdge, numRefined)
	for pair, w := range pairWeights {
		adj[pair.u] = append(adj[pair.u], partnerEdge{partner: pair.v, ochiai: w})
		adj[pair.v] = append(adj[pair.v], partnerEdge{partner: pair.u, ochiai: w})
	}
	for r := range numRefined {
		slices.SortFunc(adj[r], func(a, b partnerEdge) int {
			return cmp.Compare(a.partner, b.partner)
		})
	}

	return &leidenGraph{
		n:        numRefined,
		adj:      adj,
		strength: strength,
		seedRank: seedRank,
		twoM:     g.twoM,
	}, initComm, refComm
}
