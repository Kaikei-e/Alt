//! Graph-based Label Propagation の実装。
//! 密度が足りずクラスタリングできない記事を救済するため、グラフベースのラベル伝播を実装。

use std::collections::{HashMap, HashSet};
use tracing;

use anyhow::Result;
use ndarray::Array1;
use petgraph::{Graph, Undirected, graph::NodeIndex};
use sprs::CsMat;

use crate::classification::{Article, FeatureVector};

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
        // ラベル付き記事数をカウント
        let labeled_count = articles.iter().filter(|a| !a.genres.is_empty()).count();

        // グラフ構築開始時のログ
        tracing::info!(
            "GraphPropagator: building graph: total_articles={}, labeled_articles={}, centroid_candidates={}, similarity_threshold={:.2}",
            articles.len(),
            labeled_count,
            centroid_candidates.len(),
            self.similarity_threshold
        );

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

        // グラフ構築完了時のログ
        let node_count = self.graph.node_count();
        let edge_count = self.graph.edge_count();
        let avg_degree = if node_count > 0 {
            (edge_count as f32 * 2.0) / node_count as f32
        } else {
            0.0
        };
        let labeled_nodes_count = self
            .graph
            .node_indices()
            .filter(|&idx| self.graph[idx].is_labeled)
            .count();

        tracing::info!(
            "GraphPropagator: graph built: nodes={}, edges={}, avg_degree={:.2}, labeled_nodes={}",
            node_count,
            edge_count,
            avg_degree,
            labeled_nodes_count
        );

        Ok(())
    }

    /// ラベル伝播を実行する。
    /// 既知のラベルを持つノードから、1ホップ（直近の類似記事）のみ伝播させる。
    ///
    /// # Returns
    /// 記事IDから伝播されたラベルのマッピング
    pub fn propagate_labels(&self) -> HashMap<String, String> {
        let mut propagated_labels = HashMap::new();
        let mut labeled_sources_count = 0;

        // 既知のラベルを持つノードから開始
        for node_idx in self.graph.node_indices() {
            let node = &self.graph[node_idx];
            if node.is_labeled {
                labeled_sources_count += 1;
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

        // ラベル伝播結果のログ
        tracing::info!(
            "GraphPropagator: label propagation completed: propagated_count={}, labeled_sources={}, total_nodes={}",
            propagated_labels.len(),
            labeled_sources_count,
            self.graph.node_count()
        );

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

    /// Random Walk with Restart (RWR) によるラベル伝播
    ///
    /// # Arguments
    /// * `seeds` - シードノード（Golden Data）のインデックス
    /// * `restart_prob` - Restart確率（デフォルト: 0.15）
    /// * `max_iter` - 最大反復回数（デフォルト: 20）
    /// * `epsilon` - 収束判定閾値（デフォルト: 1e-6）
    ///
    /// # Returns
    /// 各ノードのランクスコア（確率分布）
    ///
    /// # Note
    /// グラフ構造全体を考慮したラベル伝播を行う。
    /// 数式: r = c * e + (1-c) * W^T * r
    /// ここで c は restart_prob、W は行正規化された隣接行列
    pub fn random_walk_with_restart(
        &self,
        seeds: &[NodeIndex],
        restart_prob: Option<f32>,
        max_iter: Option<usize>,
        epsilon: Option<f32>,
    ) -> HashMap<NodeIndex, f32> {
        let restart_prob = restart_prob.unwrap_or(0.15);
        let max_iter = max_iter.unwrap_or(20);
        let epsilon = epsilon.unwrap_or(1e-6);
        let n = self.graph.node_count();

        if n == 0 || seeds.is_empty() {
            return HashMap::new();
        }

        // 1. petgraphのグラフからCSR形式の隣接行列を構築
        let adj_matrix = self.build_adjacency_matrix_csr();

        // 2. 初期ベクトル e の作成（シードノードに均等に重みを分配）
        let mut r_init = vec![0.0; n];
        let seed_weight = 1.0 / seeds.len() as f32;
        for &seed in seeds {
            if let Some(idx) = self.node_index_to_matrix_index(seed) {
                r_init[idx] = seed_weight;
            }
        }

        // 3. Power Iteration
        let mut r = r_init.clone();
        for _ in 0..max_iter {
            // 伝播ステップ: (1-c) * W^T * r
            // sprs 0.11ではmul_vecが存在しないため、手動で行列ベクトル積を計算
            // W^T * r を計算（転置行列とベクトルの積）
            let adj_transposed = adj_matrix.transpose_view();
            let mut propagated = vec![0.0; n];

            // CSR形式の転置行列とベクトルの積を手動で計算
            // sprs 0.11では、outer_iterator()を使用して各行を反復処理
            for (row, outer) in adj_transposed.outer_iterator().enumerate() {
                // CsVecのindices()とdata()を使って反復処理
                let indices = outer.indices();
                let data = outer.data();
                for (idx, &col) in indices.iter().enumerate() {
                    if let Some(&val) = data.get(idx) {
                        if col < r.len() {
                            propagated[row] += val * r[col];
                        }
                    }
                }
            }

            // Restartステップとの合成: r_next = c * e + (1-c) * propagated
            let mut next_r = vec![0.0; n];
            let mut diff = 0.0;

            for i in 0..n {
                let val = restart_prob * r_init[i] + (1.0 - restart_prob) * propagated[i];
                diff += (val - r[i]).abs(); // L1ノルムでの収束判定
                next_r[i] = val;
            }

            r = next_r;

            // 収束判定
            if diff < epsilon {
                break;
            }
        }

        // 4. 結果をNodeIndexのマップに変換
        let mut result = HashMap::new();
        for node_idx in self.graph.node_indices() {
            if let Some(matrix_idx) = self.node_index_to_matrix_index(node_idx) {
                if matrix_idx < r.len() {
                    result.insert(node_idx, r[matrix_idx]);
                }
            }
        }

        result
    }

    /// petgraphのグラフからCSR形式の隣接行列を構築する
    fn build_adjacency_matrix_csr(&self) -> CsMat<f32> {
        let n = self.graph.node_count();
        let mut indptr = Vec::with_capacity(n + 1);
        let mut indices = Vec::new();
        let mut data = Vec::new();

        let mut current_ptr = 0;
        indptr.push(current_ptr);

        // ノードインデックスから行列インデックスへのマッピングを作成
        let node_to_matrix: HashMap<NodeIndex, usize> = self
            .graph
            .node_indices()
            .enumerate()
            .map(|(i, node)| (node, i))
            .collect();

        for node_idx in self.graph.node_indices() {
            let neighbors: Vec<_> = self.graph.neighbors(node_idx).collect();
            let degree = neighbors.len() as f32;

            if degree > 0.0 {
                let weight = 1.0 / degree; // 行正規化
                for neighbor in neighbors {
                    if let Some(&neighbor_matrix_idx) = node_to_matrix.get(&neighbor) {
                        indices.push(neighbor_matrix_idx);
                        data.push(weight);
                        current_ptr += 1;
                    }
                }
            }
            indptr.push(current_ptr);
        }

        // CSR形式で行列を作成
        CsMat::new((n, n), indptr, indices, data)
    }

    /// NodeIndexを行列インデックスに変換する
    fn node_index_to_matrix_index(&self, node_idx: NodeIndex) -> Option<usize> {
        // ノードインデックスから行列インデックスへのマッピングを作成
        let node_to_matrix: HashMap<NodeIndex, usize> = self
            .graph
            .node_indices()
            .enumerate()
            .map(|(i, node)| (node, i))
            .collect();
        node_to_matrix.get(&node_idx).copied()
    }

    /// 指定された特徴ベクトルに近いノードを探し、ラベルを予測する（k-NN）。
    /// Rescue Passで使用される。
    pub fn predict_by_neighbors(
        &self,
        target_vector: &FeatureVector,
        k: usize,
        thresholds: &HashMap<String, f32>,
    ) -> Option<(String, f32)> {
        let target_combined = Self::combine_feature_vector(target_vector);
        let target_norm = Self::normalize_vector(&target_combined);

        let mut neighbors: Vec<(String, f32)> = Vec::new();
        let mut candidate_count = 0;
        let mut top_similarities: Vec<(String, String, f32)> = Vec::new();

        // 全ノードとの類似度を計算
        for node_idx in self.graph.node_indices() {
            let node = &self.graph[node_idx];

            // ラベル付きノードのみ対象
            if let Some(label) = &node.label {
                let similarity = target_norm.dot(&Self::normalize_vector(&node.feature_vector));

                // 動的閾値を取得（デフォルトは0.3）
                let threshold = thresholds.get(label).copied().unwrap_or(0.3);

                // 閾値を超えた候補のみ処理
                if similarity >= threshold {
                    neighbors.push((label.clone(), similarity));
                    candidate_count += 1;

                    // デバッグ用：上位10件のみ保持（メモリ効率化）
                    top_similarities.push((node.article_id.clone(), label.clone(), similarity));
                    if top_similarities.len() > 10 {
                        top_similarities.sort_by(|a, b| {
                            b.2.partial_cmp(&a.2).unwrap_or(std::cmp::Ordering::Equal)
                        });
                        top_similarities.truncate(10);
                    }
                }
            }
        }

        // 類似度順にソート
        neighbors.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

        // デバッグ用：候補が見つかった場合のみ、上位候補をログ出力（debugレベル）
        if candidate_count > 0 && !top_similarities.is_empty() {
            top_similarities
                .sort_by(|a, b| b.2.partial_cmp(&a.2).unwrap_or(std::cmp::Ordering::Equal));
            let top_3: Vec<_> = top_similarities.iter().take(3).collect();
            tracing::warn!(
                "Rescue Pass: found {} candidates above threshold, top 3: {:?}",
                candidate_count,
                top_3
                    .iter()
                    .map(|(id, label, sim)| format!("{}:{}={:.4}", id, label, sim))
                    .collect::<Vec<_>>()
            );
        }

        // 上位k件を取得して投票
        let top_k = neighbors.iter().take(k);
        let mut votes: HashMap<String, f32> = HashMap::new();

        for (label, score) in top_k {
            *votes.entry(label.clone()).or_default() += score;
        }

        // 最もスコアが高いラベルを返す
        votes
            .into_iter()
            .max_by(|a, b| a.1.partial_cmp(&b.1).unwrap_or(std::cmp::Ordering::Equal))
    }
}

impl Default for GraphPropagator {
    /// デフォルトのGraphPropagatorを作成する（閾値0.85）。
    fn default() -> Self {
        Self::new(0.85)
    }
}
