//! Graph-based Label Propagation の実装。
//! 密度が足りずクラスタリングできない記事を救済するため、グラフベースのラベル伝播を実装。

use std::collections::{HashMap, HashSet};

use anyhow::Result;
use ndarray::Array1;
use petgraph::{Graph, Undirected, graph::NodeIndex};

use crate::classification::FeatureVector;

use super::centroid::Article;

/// グラフノードのデータ
#[derive(Debug, Clone)]
struct GraphNode {
    article_id: String,
    feature_vector: Array1<f32>,
    label: Option<String>, // 既知のラベル（Golden Dataから）
    is_labeled: bool,      // ラベルが既知かどうか
}

/// Graph Label Propagator
/// 類似記事間でラベルを伝播させる。
#[derive(Debug)]
pub struct GraphPropagator {
    /// 高信頼閾値（エッジ作成用）
    similarity_threshold: f32,
    /// グラフ
    graph: Graph<GraphNode, f32, Undirected>,
    /// 記事IDからノードインデックスのマッピング
    node_index_map: HashMap<String, NodeIndex>,
}

impl GraphPropagator {
    /// 新しいGraphPropagatorを作成する。
    #[must_use]
    pub fn new(similarity_threshold: f32) -> Self {
        Self {
            similarity_threshold,
            graph: Graph::new_undirected(),
            node_index_map: HashMap::new(),
        }
    }

    /// 記事をグラフに追加する。
    /// Centroid Classifierで候補を絞った上で、類似度が閾値を超える記事間にエッジを張る。
    ///
    /// # Arguments
    /// * `articles` - 全記事（Labeled + Unlabeled）
    /// * `centroid_candidates` - Centroid Classifierで候補に絞られた記事IDのセット
    pub fn build_graph(
        &mut self,
        articles: &[Article],
        centroid_candidates: &HashSet<String>,
    ) -> Result<()> {
        // まず全ノードを追加
        for article in articles {
            if let Some(ref feature_vector) = article.feature_vector {
                let combined = Self::combine_feature_vector(feature_vector);
                let normalized = Self::normalize_vector(&combined);

                let is_labeled = !article.genres.is_empty();
                let label = if is_labeled {
                    // 複数ジャンルがある場合は最初のものを使用
                    article.genres.first().cloned()
                } else {
                    None
                };

                let node = GraphNode {
                    article_id: article.id.clone(),
                    feature_vector: normalized,
                    label: label.clone(),
                    is_labeled,
                };

                let node_idx = self.graph.add_node(node);
                self.node_index_map.insert(article.id.clone(), node_idx);
            }
        }

        // エッジを作成（候補を絞った上で類似度計算）
        let node_indices: Vec<NodeIndex> = self.graph.node_indices().collect();
        let node_count = node_indices.len();

        // ノードのデータを事前に取得（借用チェッカーエラーを回避）
        let node_data: Vec<(NodeIndex, String, Array1<f32>, bool)> = node_indices
            .iter()
            .map(|&idx| {
                let node = &self.graph[idx];
                (
                    idx,
                    node.article_id.clone(),
                    node.feature_vector.clone(),
                    node.is_labeled,
                )
            })
            .collect();

        // O(N^2)を避けるため、Centroid Classifierで候補に絞られた記事のみを考慮
        for i in 0..node_count {
            let (node_i, article_id_i, feature_vector_i, is_labeled_i) = &node_data[i];

            // 候補に含まれていない場合はスキップ（ただし、ラベル付きは常に考慮）
            if !is_labeled_i && !centroid_candidates.contains(article_id_i) {
                continue;
            }

            // 既にラベルがある場合は、エッジを作成するだけで伝播は不要
            // ただし、他の未ラベル記事へのエッジは作成する
            for (node_j, article_id_j, feature_vector_j, is_labeled_j) in
                node_data.iter().skip(i + 1)
            {
                // 両方ラベル付きの場合はエッジを作成しない（伝播不要）
                if *is_labeled_i && *is_labeled_j {
                    continue;
                }

                // 少なくとも片方が候補に含まれているか、ラベル付きである必要がある
                let i_is_candidate = *is_labeled_i || centroid_candidates.contains(article_id_i);
                let j_is_candidate = *is_labeled_j || centroid_candidates.contains(article_id_j);

                if !i_is_candidate && !j_is_candidate {
                    continue;
                }

                // コサイン類似度を計算
                let similarity = feature_vector_i.dot(feature_vector_j);

                // 閾値を超える場合のみエッジを追加
                if similarity >= self.similarity_threshold {
                    self.graph.add_edge(*node_i, *node_j, similarity);
                }
            }
        }

        Ok(())
    }

    /// ラベル伝播を実行する。
    /// 既知のラベルを持つノードから、1ホップ（直近の類似記事）のみ伝播させる。
    ///
    /// # Returns
    /// 記事IDから伝播されたラベルのマッピング
    pub fn propagate_labels(&self) -> HashMap<String, String> {
        let mut propagated_labels = HashMap::new();

        // 既知のラベルを持つノードから開始
        for node_idx in self.graph.node_indices() {
            let node = &self.graph[node_idx];
            if node.is_labeled {
                if let Some(ref label) = node.label {
                    // 1ホップの隣接ノードに伝播
                    for neighbor_idx in self.graph.neighbors(node_idx) {
                        let neighbor = &self.graph[neighbor_idx];

                        // 既にラベルがある場合は上書きしない
                        if !neighbor.is_labeled {
                            propagated_labels.insert(neighbor.article_id.clone(), label.clone());
                        }
                    }
                }
            }
        }

        propagated_labels
    }

    /// FeatureVectorを結合して1つのベクトルに変換する。
    fn combine_feature_vector(feature_vector: &FeatureVector) -> Array1<f32> {
        let total_dim =
            feature_vector.tfidf.len() + feature_vector.bm25.len() + feature_vector.embedding.len();
        let mut combined = Vec::with_capacity(total_dim);

        combined.extend_from_slice(&feature_vector.tfidf);
        combined.extend_from_slice(&feature_vector.bm25);
        combined.extend_from_slice(&feature_vector.embedding);

        Array1::from_vec(combined)
    }

    /// ベクトルをL2正規化する。
    fn normalize_vector(vector: &Array1<f32>) -> Array1<f32> {
        let norm = vector.dot(vector).sqrt();
        if norm > 0.0 {
            vector / norm
        } else {
            vector.clone()
        }
    }

    /// グラフの統計情報を取得する（デバッグ用）。
    #[allow(dead_code)]
    pub fn graph_stats(&self) -> (usize, usize) {
        (self.graph.node_count(), self.graph.edge_count())
    }
}

impl Default for GraphPropagator {
    /// デフォルトのGraphPropagatorを作成する（閾値0.85）。
    fn default() -> Self {
        Self::new(0.85)
    }
}
