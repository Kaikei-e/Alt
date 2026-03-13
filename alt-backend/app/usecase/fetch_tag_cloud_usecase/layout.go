package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"math"
	"math/rand"
)

const (
	layoutIterations   = 300
	repulsionConstant  = 150.0
	attractionConstant = 0.0003
	centeringForce     = 0.0001
	dampingFactor      = 0.9
	minDistance         = 1.0
	positionBound      = 100.0
	layoutSeed         = 42

	// Early convergence: stop when max displacement stays below threshold
	// for stableRuns consecutive iterations.
	convergenceRatio = 0.005 // relative to initialRadius
	stableRuns       = 5
)

type node struct {
	x, y, z    float64
	vx, vy, vz float64
}

// ComputeLayout computes 3D positions for tags using a force-directed graph layout.
// Positions are written directly to the items' PositionX/Y/Z fields.
// The algorithm is deterministic (seeded RNG).
func ComputeLayout(items []*domain.TagCloudItem, edges []*domain.TagCooccurrence) {
	computeLayoutInternal(items, edges, nil)
}

// ComputeLayoutWithProfile computes the layout and returns the max displacement per iteration.
// Used for convergence analysis and benchmark tuning.
func ComputeLayoutWithProfile(items []*domain.TagCloudItem, edges []*domain.TagCooccurrence) []float64 {
	profile := make([]float64, 0, layoutIterations)
	computeLayoutInternal(items, edges, &profile)
	return profile
}

// computeLayoutInternal is the shared implementation.
// If profile is non-nil, max displacement per iteration is appended to it.
func computeLayoutInternal(items []*domain.TagCloudItem, edges []*domain.TagCooccurrence, profile *[]float64) {
	n := len(items)
	if n == 0 {
		return
	}
	if n == 1 {
		items[0].PositionX = 0
		items[0].PositionY = 0
		items[0].PositionZ = 0
		return
	}

	// Build name-to-index map
	nameToIdx := make(map[string]int, n)
	for i, item := range items {
		nameToIdx[item.TagName] = i
	}

	// Build edge list with indices
	type edge struct {
		from, to int
		weight   float64
	}
	var edgeList []edge
	for _, e := range edges {
		fromIdx, ok1 := nameToIdx[e.TagNameA]
		toIdx, ok2 := nameToIdx[e.TagNameB]
		if ok1 && ok2 {
			edgeList = append(edgeList, edge{from: fromIdx, to: toIdx, weight: float64(e.SharedCount)})
		}
	}

	// Initialize positions on sphere surface (deterministic)
	rng := rand.New(rand.NewSource(layoutSeed))
	initialRadius := math.Sqrt(float64(n)) * 8.0
	nodes := make([]node, n)
	for i := range nodes {
		// Uniform random on sphere surface
		theta := rng.Float64() * 2 * math.Pi
		phi := math.Acos(2*rng.Float64() - 1)
		r := initialRadius * math.Cbrt(rng.Float64()) // volumetric distribution
		nodes[i] = node{
			x: r * math.Sin(phi) * math.Cos(theta),
			y: r * math.Sin(phi) * math.Sin(theta),
			z: r * math.Cos(phi),
		}
	}

	// Force-directed iterations with early convergence
	convergenceThreshold := initialRadius * convergenceRatio
	stableCount := 0
	for iter := range layoutIterations {
		temperature := 1.0 - float64(iter)/float64(layoutIterations) // cooling
		maxDisplacement := initialRadius * 0.1 * temperature

		// Reset forces (stored in velocity)
		for i := range nodes {
			nodes[i].vx = 0
			nodes[i].vy = 0
			nodes[i].vz = 0
		}

		// Repulsion via Barnes-Hut approximation O(n log n)
		tree := buildOctree(nodes)
		if tree != nil {
			for i := range nodes {
				fx, fy, fz := tree.computeForce(i, nodes[i].x, nodes[i].y, nodes[i].z, barnesHutTheta)
				nodes[i].vx += fx
				nodes[i].vy += fy
				nodes[i].vz += fz
			}
		}

		// Attraction along edges (spring-like)
		for _, e := range edgeList {
			dx := nodes[e.to].x - nodes[e.from].x
			dy := nodes[e.to].y - nodes[e.from].y
			dz := nodes[e.to].z - nodes[e.from].z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist < minDistance {
				dist = minDistance
			}
			// Attraction proportional to distance and edge weight
			weightFactor := math.Log2(e.weight + 1)
			force := attractionConstant * dist * weightFactor
			fx := force * dx / dist
			fy := force * dy / dist
			fz := force * dz / dist
			nodes[e.from].vx += fx
			nodes[e.from].vy += fy
			nodes[e.from].vz += fz
			nodes[e.to].vx -= fx
			nodes[e.to].vy -= fy
			nodes[e.to].vz -= fz
		}

		// Centering force (pull toward origin)
		for i := range nodes {
			nodes[i].vx -= centeringForce * nodes[i].x
			nodes[i].vy -= centeringForce * nodes[i].y
			nodes[i].vz -= centeringForce * nodes[i].z
		}

		// Apply forces with damping and max displacement
		iterMaxDisp := 0.0
		for i := range nodes {
			nodes[i].vx *= dampingFactor
			nodes[i].vy *= dampingFactor
			nodes[i].vz *= dampingFactor

			// Limit displacement
			disp := math.Sqrt(nodes[i].vx*nodes[i].vx + nodes[i].vy*nodes[i].vy + nodes[i].vz*nodes[i].vz)
			if disp > maxDisplacement {
				scale := maxDisplacement / disp
				nodes[i].vx *= scale
				nodes[i].vy *= scale
				nodes[i].vz *= scale
				disp = maxDisplacement
			}
			iterMaxDisp = max(iterMaxDisp, disp)

			nodes[i].x += nodes[i].vx
			nodes[i].y += nodes[i].vy
			nodes[i].z += nodes[i].vz
		}

		// Record max displacement for convergence profiling
		if profile != nil {
			*profile = append(*profile, iterMaxDisp)
		}

		// Early convergence: stop when max displacement stays below threshold
		if iterMaxDisp < convergenceThreshold {
			stableCount++
			if stableCount >= stableRuns {
				break
			}
		} else {
			stableCount = 0
		}
	}

	// Normalize positions to fit within bounds
	maxCoord := 0.0
	for _, nd := range nodes {
		maxCoord = max(maxCoord, math.Abs(nd.x))
		maxCoord = max(maxCoord, math.Abs(nd.y))
		maxCoord = max(maxCoord, math.Abs(nd.z))
	}
	if maxCoord > 0 {
		scale := positionBound / maxCoord
		for i, nd := range nodes {
			items[i].PositionX = nd.x * scale
			items[i].PositionY = nd.y * scale
			items[i].PositionZ = nd.z * scale
		}
	}
}
