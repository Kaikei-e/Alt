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
	maxLPARounds         = 30
	minCommunityTagCount = 4
	maxCommunities       = 8
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

// ComputeEdges selects each tag's 3 strongest partners by Ochiai coefficient,
// takes the deduplicated undirected union (source_index < target_index), caps
// at the 600 strongest, normalises weights to (0, 1] by the maximum kept
// Ochiai, and sorts by weight descending, then source_index, then target_index.
func ComputeEdges(items []*domain.TagCloudItem, cooccurrences []*domain.TagCooccurrence) []*domain.TagCloudEdge {
	n := len(items)
	if n < 2 || len(cooccurrences) == 0 {
		return nil
	}

	nameToIdx := make(map[string]int, n)
	for i, item := range items {
		nameToIdx[item.TagName] = i
	}

	pairOchiai := make(map[indexPair]float64, len(cooccurrences))
	for _, c := range cooccurrences {
		if c.SharedCount <= 0 {
			continue
		}
		idxA, okA := nameToIdx[c.TagNameA]
		idxB, okB := nameToIdx[c.TagNameB]
		if !okA || !okB || idxA == idxB {
			continue
		}
		countA := items[idxA].ArticleCount
		countB := items[idxB].ArticleCount
		if countA <= 0 || countB <= 0 {
			continue
		}
		ochiai := float64(c.SharedCount) / math.Sqrt(float64(countA)*float64(countB))
		if ochiai <= 0 {
			continue
		}
		u, v := idxA, idxB
		if u > v {
			u, v = v, u
		}
		key := indexPair{u: u, v: v}
		if prev, exists := pairOchiai[key]; !exists || ochiai > prev {
			pairOchiai[key] = ochiai
		}
	}

	if len(pairOchiai) == 0 {
		return nil
	}

	adj := make([][]partnerEdge, n)
	for pair, w := range pairOchiai {
		adj[pair.u] = append(adj[pair.u], partnerEdge{partner: pair.v, ochiai: w})
		adj[pair.v] = append(adj[pair.v], partnerEdge{partner: pair.u, ochiai: w})
	}

	keptMap := make(map[indexPair]float64)
	for i := range n {
		partners := adj[i]
		if len(partners) == 0 {
			continue
		}
		slices.SortFunc(partners, func(a, b partnerEdge) int {
			if c := cmp.Compare(b.ochiai, a.ochiai); c != 0 {
				return c
			}
			return cmp.Compare(a.partner, b.partner)
		})
		limit := min(len(partners), maxPartnersPerTag)
		for _, p := range partners[:limit] {
			u, v := i, p.partner
			if u > v {
				u, v = v, u
			}
			keptMap[indexPair{u: u, v: v}] = p.ochiai
		}
	}

	if len(keptMap) == 0 {
		return nil
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

// ComputeClusters assigns topic communities via deterministic weighted label
// propagation over all association pairs. Communities with fewer than 4 tags
// receive cluster 0, and up to the 8 largest communities by total article
// count receive clusters 1..8.
func ComputeClusters(items []*domain.TagCloudItem, cooccurrences []*domain.TagCooccurrence) {
	n := len(items)
	for _, item := range items {
		item.ClusterID = 0
	}
	if n < minCommunityTagCount || len(cooccurrences) == 0 {
		return
	}

	// Rank tags in the fixed order (article count descending, then tag name
	// ascending) so label propagation is invariant to input slice order.
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
		if ochiai <= 0 {
			continue
		}
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
		return
	}

	adj := make([][]partnerEdge, n)
	for pair, w := range pairOchiai {
		adj[pair.u] = append(adj[pair.u], partnerEdge{partner: pair.v, ochiai: w})
		adj[pair.v] = append(adj[pair.v], partnerEdge{partner: pair.u, ochiai: w})
	}
	// Sort neighbour lists by fixed-order rank so floating-point summation is
	// independent of map iteration order.
	for r := range n {
		slices.SortFunc(adj[r], func(a, b partnerEdge) int {
			return cmp.Compare(a.partner, b.partner)
		})
	}

	labels := make([]int, n)
	for r := range n {
		labels[r] = r
	}

	for range maxLPARounds {
		changed := false
		for r := range n {
			neighbours := adj[r]
			if len(neighbours) == 0 {
				continue
			}

			labelWeights := make(map[int]float64, len(neighbours))
			for _, nb := range neighbours {
				labelWeights[labels[nb.partner]] += nb.ochiai
			}

			bestLabel := -1
			bestWeight := -1.0
			for lbl, w := range labelWeights {
				if w > bestWeight || (w == bestWeight && lbl < bestLabel) {
					bestWeight = w
					bestLabel = lbl
				}
			}

			if bestLabel >= 0 && bestLabel != labels[r] {
				labels[r] = bestLabel
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	type community struct {
		seedRank     int
		memberRanks  []int
		totalArticle int
	}

	byLabel := make(map[int]*community)
	for r, lbl := range labels {
		comm, ok := byLabel[lbl]
		if !ok {
			comm = &community{seedRank: lbl}
			byLabel[lbl] = comm
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
