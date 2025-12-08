use rand::seq::SliceRandom;
use rand::thread_rng;

/// Simple K-Means clustering implementation for 1D vectors (embeddings).
pub struct KMeans {
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

fn distance_sq(a: &[f32], b: &[f32]) -> f32 {
    a.iter().zip(b.iter()).map(|(x, y)| (x - y).powi(2)).sum()
}
