use rand::seq::SliceRandom;
use rand::thread_rng;

/// Simple K-Means clustering implementation for 1D vectors (embeddings).
pub struct KMeans {
    pub centroids: Vec<Vec<f32>>,
    pub assignments: Vec<usize>,
}

pub struct MiniBatchKMeans {
    pub centroids: Vec<Vec<f32>>,
    pub assignments: Vec<usize>,
}

impl KMeans {
    /// Runs K-Means clustering.
    ///
    /// # Arguments
    /// * `data` - List of data points (vectors).
    /// * `k` - Number of clusters.
    /// * `max_iterations` - Maximum number of iterations.
    pub fn new(data: &[Vec<f32>], k: usize, max_iterations: usize) -> Self {
        if data.is_empty() || k == 0 {
            return Self {
                centroids: vec![],
                assignments: vec![],
            };
        }

        let k = k.min(data.len());
        let dim = data[0].len();
        let mut rng = thread_rng();

        // Initialize centroids randomly
        let mut centroids: Vec<Vec<f32>> = data.choose_multiple(&mut rng, k).cloned().collect();

        let mut assignments = vec![0; data.len()];
        let mut changes = true;
        let mut iterations = 0;

        while changes && iterations < max_iterations {
            changes = false;
            iterations += 1;

            // E-step: Assign points to nearest centroid
            let mut new_assignments = vec![0; data.len()];
            for (i, point) in data.iter().enumerate() {
                let mut min_dist_sq = f32::MAX;
                let mut best_cluster = 0;

                for (j, centroid) in centroids.iter().enumerate() {
                    let dist_sq = distance_sq(point, centroid);
                    if dist_sq < min_dist_sq {
                        min_dist_sq = dist_sq;
                        best_cluster = j;
                    }
                }
                new_assignments[i] = best_cluster;
            }

            if new_assignments != assignments {
                assignments = new_assignments;
                changes = true;
            }

            // M-step: Update centroids
            let mut sums = vec![vec![0.0; dim]; k];
            let mut counts = vec![0; k];

            for (i, &cluster) in assignments.iter().enumerate() {
                for (j, val) in data[i].iter().enumerate() {
                    sums[cluster][j] += val;
                }
                counts[cluster] += 1;
            }

            for j in 0..k {
                if counts[j] > 0 {
                    for l in 0..dim {
                        centroids[j][l] = sums[j][l] / counts[j] as f32;
                    }
                } else {
                    // Re-initialize empty cluster with a random point (optional, keeps robustness)
                    if let Some(random_point) = data.choose(&mut rng) {
                        centroids[j].clone_from(random_point);
                    }
                }
            }
        }

        Self {
            centroids,
            assignments,
        }
    }
}

impl MiniBatchKMeans {
    /// Runs mini-batch K-Means using online centroid updates.
    ///
    /// This follows the standard mini-batch update pattern described by
    /// Sculley (2010): shuffle the dataset, update centroids from small
    /// batches, then assign all samples in a final pass.
    pub fn new(data: &[Vec<f32>], k: usize, max_iterations: usize, batch_size: usize) -> Self {
        if data.is_empty() || k == 0 {
            return Self {
                centroids: vec![],
                assignments: vec![],
            };
        }

        let k = k.min(data.len());
        let dim = data[0].len();
        let mut rng = thread_rng();
        let mut centroids: Vec<Vec<f32>> = data.choose_multiple(&mut rng, k).cloned().collect();
        let mut counts = vec![0usize; k];
        let mut indices: Vec<usize> = (0..data.len()).collect();
        let batch_size = batch_size.clamp(1, data.len());
        let epochs = max_iterations.max(1);

        for _ in 0..epochs {
            indices.shuffle(&mut rng);

            for batch in indices.chunks(batch_size) {
                for &idx in batch {
                    let point = &data[idx];
                    let cluster = nearest_centroid(point, &centroids);
                    counts[cluster] += 1;
                    let eta = 1.0 / counts[cluster] as f32;

                    for axis in 0..dim {
                        centroids[cluster][axis] =
                            (1.0 - eta) * centroids[cluster][axis] + eta * point[axis];
                    }
                }
            }
        }

        let assignments = data
            .iter()
            .map(|point| nearest_centroid(point, &centroids))
            .collect();

        Self {
            centroids,
            assignments,
        }
    }
}

fn distance_sq(a: &[f32], b: &[f32]) -> f32 {
    a.iter().zip(b.iter()).map(|(x, y)| (x - y).powi(2)).sum()
}

fn nearest_centroid(point: &[f32], centroids: &[Vec<f32>]) -> usize {
    let mut min_dist_sq = f32::MAX;
    let mut best_cluster = 0;

    for (cluster_idx, centroid) in centroids.iter().enumerate() {
        let dist_sq = distance_sq(point, centroid);
        if dist_sq < min_dist_sq {
            min_dist_sq = dist_sq;
            best_cluster = cluster_idx;
        }
    }

    best_cluster
}

#[cfg(test)]
mod tests {
    use super::MiniBatchKMeans;

    #[test]
    fn mini_batch_kmeans_assigns_every_point() {
        let data = vec![
            vec![0.0, 0.0],
            vec![0.1, 0.1],
            vec![10.0, 10.0],
            vec![10.1, 10.1],
        ];

        let kmeans = MiniBatchKMeans::new(&data, 2, 10, 2);

        assert_eq!(kmeans.assignments.len(), data.len());
        assert_eq!(kmeans.centroids.len(), 2);
        assert!(kmeans.assignments.iter().all(|cluster| *cluster < 2));
    }
}
