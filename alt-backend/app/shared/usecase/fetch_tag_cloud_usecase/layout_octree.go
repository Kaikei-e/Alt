package fetch_tag_cloud_usecase

import "math"

const barnesHutTheta = 1.0 // Opening parameter: balance of speed/accuracy

// octreeNode represents a node in a 3D Barnes-Hut octree.
type octreeNode struct {
	// Spatial bounds
	centerX, centerY, centerZ float64
	halfSize                  float64

	// Center of mass
	totalMass        float64
	comX, comY, comZ float64

	// Body info
	bodyCount           int
	bodyIndex           int // valid only for leaf with exactly 1 body
	bodyX, bodyY, bodyZ float64

	// Children (8 octants)
	children [8]*octreeNode
	isLeaf   bool
}

// newOctree creates a root octree node centered at (cx, cy, cz) with the given halfSize.
func newOctree(cx, cy, cz, halfSize float64) *octreeNode {
	return &octreeNode{
		centerX:   cx,
		centerY:   cy,
		centerZ:   cz,
		halfSize:  halfSize,
		isLeaf:    true,
		bodyIndex: -1,
	}
}

// octantIndex returns which of the 8 octants (x, y, z) falls into.
func (n *octreeNode) octantIndex(x, y, z float64) int {
	idx := 0
	if x >= n.centerX {
		idx |= 1
	}
	if y >= n.centerY {
		idx |= 2
	}
	if z >= n.centerZ {
		idx |= 4
	}
	return idx
}

// childCenter returns the center of the specified octant child.
func (n *octreeNode) childCenter(octant int) (float64, float64, float64) {
	qs := n.halfSize / 2 // quarter size
	cx := n.centerX
	cy := n.centerY
	cz := n.centerZ
	if octant&1 != 0 {
		cx += qs
	} else {
		cx -= qs
	}
	if octant&2 != 0 {
		cy += qs
	} else {
		cy -= qs
	}
	if octant&4 != 0 {
		cz += qs
	} else {
		cz -= qs
	}
	return cx, cy, cz
}

// insert adds a body at position (x, y, z) with the given index.
func (n *octreeNode) insert(index int, x, y, z float64) {
	if n.bodyCount == 0 {
		// Empty leaf → store body
		n.bodyIndex = index
		n.bodyX = x
		n.bodyY = y
		n.bodyZ = z
		n.bodyCount = 1
		n.totalMass = 1.0
		n.comX = x
		n.comY = y
		n.comZ = z
		n.isLeaf = true
		return
	}

	if n.isLeaf && n.bodyCount == 1 {
		// Leaf with 1 body → subdivide
		oldIdx := n.bodyIndex
		oldX, oldY, oldZ := n.bodyX, n.bodyY, n.bodyZ
		n.isLeaf = false
		n.bodyIndex = -1

		// Re-insert the existing body into appropriate child
		n.insertIntoChild(oldIdx, oldX, oldY, oldZ)
	}

	// Insert new body into appropriate child
	n.insertIntoChild(index, x, y, z)

	// Update center of mass
	n.bodyCount++
	n.totalMass = float64(n.bodyCount)
	n.comX = (n.comX*float64(n.bodyCount-1) + x) / float64(n.bodyCount)
	n.comY = (n.comY*float64(n.bodyCount-1) + y) / float64(n.bodyCount)
	n.comZ = (n.comZ*float64(n.bodyCount-1) + z) / float64(n.bodyCount)
}

// insertIntoChild inserts a body into the appropriate child octant.
func (n *octreeNode) insertIntoChild(index int, x, y, z float64) {
	oct := n.octantIndex(x, y, z)
	if n.children[oct] == nil {
		cx, cy, cz := n.childCenter(oct)
		n.children[oct] = newOctree(cx, cy, cz, n.halfSize/2)
	}
	n.children[oct].insert(index, x, y, z)
}

// computeForce computes the repulsive force on body at (px, py, pz) with given index.
// Returns (fx, fy, fz). Uses Barnes-Hut approximation with the given theta parameter.
func (n *octreeNode) computeForce(bodyIdx int, px, py, pz, theta float64) (float64, float64, float64) {
	if n.bodyCount == 0 {
		return 0, 0, 0
	}

	dx := px - n.comX
	dy := py - n.comY
	dz := pz - n.comZ
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Leaf with single body
	if n.isLeaf && n.bodyCount == 1 {
		if n.bodyIndex == bodyIdx {
			return 0, 0, 0 // Skip self
		}
		return computeDirectForce(dx, dy, dz, dist, 1.0)
	}

	// Barnes-Hut criterion: if cell is far enough, use center of mass
	cellSize := n.halfSize * 2
	if dist > 0 && cellSize/dist < theta {
		return computeDirectForce(dx, dy, dz, dist, n.totalMass)
	}

	// Otherwise recurse into children
	var fx, fy, fz float64
	for _, child := range n.children {
		if child != nil {
			cfx, cfy, cfz := child.computeForce(bodyIdx, px, py, pz, theta)
			fx += cfx
			fy += cfy
			fz += cfz
		}
	}
	return fx, fy, fz
}

// computeDirectForce computes the Coulomb-like repulsive force.
func computeDirectForce(dx, dy, dz, dist, mass float64) (float64, float64, float64) {
	if dist < minDistance {
		dist = minDistance
	}
	force := repulsionConstant * mass / (dist * dist)
	fx := force * dx / dist
	fy := force * dy / dist
	fz := force * dz / dist
	return fx, fy, fz
}

// buildOctree constructs an octree from a slice of nodes.
func buildOctree(nodes []node) *octreeNode {
	if len(nodes) == 0 {
		return nil
	}

	// Find bounding box
	maxCoord := 0.0
	for _, nd := range nodes {
		maxCoord = max(maxCoord, math.Abs(nd.x))
		maxCoord = max(maxCoord, math.Abs(nd.y))
		maxCoord = max(maxCoord, math.Abs(nd.z))
	}
	// Add padding to avoid bodies exactly on boundaries
	halfSize := maxCoord + 1.0

	tree := newOctree(0, 0, 0, halfSize)
	for i, nd := range nodes {
		tree.insert(i, nd.x, nd.y, nd.z)
	}
	return tree
}
