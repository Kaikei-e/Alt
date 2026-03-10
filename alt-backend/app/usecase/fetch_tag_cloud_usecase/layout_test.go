package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"math"
	"testing"
)

func distance(a, b *domain.TagCloudItem) float64 {
	dx := a.PositionX - b.PositionX
	dy := a.PositionY - b.PositionY
	dz := a.PositionZ - b.PositionZ
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func TestComputeLayout_EmptyInput(t *testing.T) {
	items := []*domain.TagCloudItem{}
	edges := []*domain.TagCooccurrence{}
	ComputeLayout(items, edges)
	if len(items) != 0 {
		t.Errorf("expected empty result, got %d items", len(items))
	}
}

func TestComputeLayout_SingleNode(t *testing.T) {
	items := []*domain.TagCloudItem{
		{TagName: "go", ArticleCount: 10},
	}
	ComputeLayout(items, nil)

	if items[0].PositionX != 0 || items[0].PositionY != 0 || items[0].PositionZ != 0 {
		t.Errorf("single node should be at origin, got (%f, %f, %f)",
			items[0].PositionX, items[0].PositionY, items[0].PositionZ)
	}
}

func TestComputeLayout_UnconnectedNodesSpreadApart(t *testing.T) {
	items := []*domain.TagCloudItem{
		{TagName: "go", ArticleCount: 10},
		{TagName: "rust", ArticleCount: 8},
		{TagName: "python", ArticleCount: 12},
	}
	ComputeLayout(items, nil)

	// All nodes should be separated by repulsion
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			d := distance(items[i], items[j])
			if d < 1.0 {
				t.Errorf("nodes %s and %s are too close: %f",
					items[i].TagName, items[j].TagName, d)
			}
		}
	}
}

func TestComputeLayout_ConnectedNodesCloserThanUnconnected(t *testing.T) {
	items := []*domain.TagCloudItem{
		{TagName: "go", ArticleCount: 10},
		{TagName: "rust", ArticleCount: 8},
		{TagName: "python", ArticleCount: 12},
	}
	edges := []*domain.TagCooccurrence{
		{TagNameA: "go", TagNameB: "rust", SharedCount: 20},
	}
	ComputeLayout(items, edges)

	connectedDist := distance(items[0], items[1]) // go-rust (connected)
	unconnectedDist1 := distance(items[0], items[2]) // go-python
	unconnectedDist2 := distance(items[1], items[2]) // rust-python

	avgUnconnected := (unconnectedDist1 + unconnectedDist2) / 2

	if connectedDist >= avgUnconnected {
		t.Errorf("connected pair (go-rust) distance %f should be less than average unconnected distance %f",
			connectedDist, avgUnconnected)
	}
}

func TestComputeLayout_PositionsWithinBounds(t *testing.T) {
	items := make([]*domain.TagCloudItem, 50)
	for i := range items {
		items[i] = &domain.TagCloudItem{
			TagName:      "tag" + string(rune('a'+i%26)),
			ArticleCount: i + 1,
		}
	}

	// Some random edges
	edges := []*domain.TagCooccurrence{
		{TagNameA: "taga", TagNameB: "tagb", SharedCount: 5},
		{TagNameA: "tagc", TagNameB: "tagd", SharedCount: 3},
	}
	ComputeLayout(items, edges)

	const bound = 110.0
	for _, item := range items {
		if math.Abs(item.PositionX) > bound || math.Abs(item.PositionY) > bound || math.Abs(item.PositionZ) > bound {
			t.Errorf("node %s position (%f, %f, %f) exceeds bound %f",
				item.TagName, item.PositionX, item.PositionY, item.PositionZ, bound)
		}
	}
}

func TestComputeLayout_Deterministic(t *testing.T) {
	makeItems := func() []*domain.TagCloudItem {
		return []*domain.TagCloudItem{
			{TagName: "go", ArticleCount: 10},
			{TagName: "rust", ArticleCount: 8},
			{TagName: "python", ArticleCount: 12},
		}
	}
	edges := []*domain.TagCooccurrence{
		{TagNameA: "go", TagNameB: "rust", SharedCount: 5},
	}

	items1 := makeItems()
	ComputeLayout(items1, edges)

	items2 := makeItems()
	ComputeLayout(items2, edges)

	for i := range items1 {
		if items1[i].PositionX != items2[i].PositionX ||
			items1[i].PositionY != items2[i].PositionY ||
			items1[i].PositionZ != items2[i].PositionZ {
			t.Errorf("layout is not deterministic for node %s", items1[i].TagName)
		}
	}
}
