package fetch_tag_cloud_usecase

import (
	"math"
	"testing"
)

func TestOctreeInsert_SingleBody(t *testing.T) {
	tree := newOctree(0, 0, 0, 100)
	tree.insert(0, 10, 20, 30)

	if tree.bodyCount != 1 {
		t.Errorf("expected bodyCount=1, got %d", tree.bodyCount)
	}
	if !tree.isLeaf {
		t.Error("single body octree should be a leaf")
	}
	if tree.comX != 10 || tree.comY != 20 || tree.comZ != 30 {
		t.Errorf("center of mass should be body position, got (%f, %f, %f)", tree.comX, tree.comY, tree.comZ)
	}
}

func TestOctreeInsert_MultipleBodies(t *testing.T) {
	tree := newOctree(0, 0, 0, 100)
	tree.insert(0, 10, 20, 30)
	tree.insert(1, -10, -20, -30)
	tree.insert(2, 50, 50, 50)

	if tree.bodyCount != 3 {
		t.Errorf("expected bodyCount=3, got %d", tree.bodyCount)
	}
	if tree.isLeaf {
		t.Error("multiple body octree should not be a leaf")
	}
	// At least one child should have bodies
	hasChild := false
	for _, c := range tree.children {
		if c != nil && c.bodyCount > 0 {
			hasChild = true
			break
		}
	}
	if !hasChild {
		t.Error("octree should have at least one child with bodies")
	}
}

func TestOctreeCenterOfMass(t *testing.T) {
	tree := newOctree(0, 0, 0, 100)
	// Two bodies at symmetric positions → center of mass at origin
	tree.insert(0, 10, 0, 0)
	tree.insert(1, -10, 0, 0)

	if tree.totalMass != 2 {
		t.Errorf("expected totalMass=2, got %f", tree.totalMass)
	}
	// Center of mass should be at (0, 0, 0)
	if math.Abs(tree.comX) > 1e-9 || math.Abs(tree.comY) > 1e-9 || math.Abs(tree.comZ) > 1e-9 {
		t.Errorf("center of mass should be near origin, got (%f, %f, %f)", tree.comX, tree.comY, tree.comZ)
	}
}

func TestOctreeForceApproximation_ThetaZero(t *testing.T) {
	// With theta=0, Barnes-Hut should compute exact same forces as naive O(n²)
	positions := []struct{ x, y, z float64 }{
		{10, 0, 0},
		{-10, 0, 0},
		{0, 15, 0},
		{0, -15, 0},
		{0, 0, 20},
	}
	n := len(positions)

	// Compute naive repulsion forces
	naiveForces := make([][3]float64, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			dx := positions[i].x - positions[j].x
			dy := positions[i].y - positions[j].y
			dz := positions[i].z - positions[j].z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist < minDistance {
				dist = minDistance
			}
			force := repulsionConstant / (dist * dist)
			fx := force * dx / dist
			fy := force * dy / dist
			fz := force * dz / dist
			naiveForces[i][0] += fx
			naiveForces[i][1] += fy
			naiveForces[i][2] += fz
			naiveForces[j][0] -= fx
			naiveForces[j][1] -= fy
			naiveForces[j][2] -= fz
		}
	}

	// Compute Barnes-Hut forces with theta=0 (exact)
	tree := newOctree(0, 0, 0, 50)
	for i, p := range positions {
		tree.insert(i, p.x, p.y, p.z)
	}

	bhForces := make([][3]float64, n)
	for i, p := range positions {
		fx, fy, fz := tree.computeForce(i, p.x, p.y, p.z, 0.0) // theta=0
		bhForces[i] = [3]float64{fx, fy, fz}
	}

	for i := 0; i < n; i++ {
		for d := 0; d < 3; d++ {
			if math.Abs(naiveForces[i][d]-bhForces[i][d]) > 1e-6 {
				t.Errorf("node %d dim %d: naive=%f, barnes-hut(theta=0)=%f",
					i, d, naiveForces[i][d], bhForces[i][d])
			}
		}
	}
}

func TestBarnesHutVsNaive_Accuracy(t *testing.T) {
	// With theta=1.0 (default), error should be within 10% of naive
	positions := make([]struct{ x, y, z float64 }, 50)
	rng := newDeterministicRNG(42)
	for i := range positions {
		positions[i].x = (rng.Float64() - 0.5) * 200
		positions[i].y = (rng.Float64() - 0.5) * 200
		positions[i].z = (rng.Float64() - 0.5) * 200
	}
	n := len(positions)

	// Naive forces
	naiveForces := make([][3]float64, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			dx := positions[i].x - positions[j].x
			dy := positions[i].y - positions[j].y
			dz := positions[i].z - positions[j].z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist < minDistance {
				dist = minDistance
			}
			force := repulsionConstant / (dist * dist)
			fx := force * dx / dist
			fy := force * dy / dist
			fz := force * dz / dist
			naiveForces[i][0] += fx
			naiveForces[i][1] += fy
			naiveForces[i][2] += fz
			naiveForces[j][0] -= fx
			naiveForces[j][1] -= fy
			naiveForces[j][2] -= fz
		}
	}

	// Barnes-Hut forces
	tree := newOctree(0, 0, 0, 150)
	for i, p := range positions {
		tree.insert(i, p.x, p.y, p.z)
	}

	totalNaiveMag := 0.0
	totalErrorMag := 0.0
	for i, p := range positions {
		fx, fy, fz := tree.computeForce(i, p.x, p.y, p.z, barnesHutTheta)
		naiveMag := math.Sqrt(naiveForces[i][0]*naiveForces[i][0] +
			naiveForces[i][1]*naiveForces[i][1] +
			naiveForces[i][2]*naiveForces[i][2])
		errX := naiveForces[i][0] - fx
		errY := naiveForces[i][1] - fy
		errZ := naiveForces[i][2] - fz
		errMag := math.Sqrt(errX*errX + errY*errY + errZ*errZ)
		totalNaiveMag += naiveMag
		totalErrorMag += errMag
	}

	relativeError := totalErrorMag / totalNaiveMag
	if relativeError > 0.10 {
		t.Errorf("Barnes-Hut relative error %f exceeds 10%% threshold", relativeError)
	}
	t.Logf("Barnes-Hut relative error: %.4f (%.2f%%)", relativeError, relativeError*100)
}

// newDeterministicRNG is a simple helper for tests
func newDeterministicRNG(seed int64) *deterministicRNG {
	return &deterministicRNG{state: uint64(seed)}
}

type deterministicRNG struct {
	state uint64
}

func (r *deterministicRNG) Float64() float64 {
	// xorshift64
	r.state ^= r.state << 13
	r.state ^= r.state >> 7
	r.state ^= r.state << 17
	return float64(r.state) / float64(math.MaxUint64)
}
