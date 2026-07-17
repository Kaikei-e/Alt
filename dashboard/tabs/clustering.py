import numpy as np
import pandas as pd
import streamlit as st
from utils import fetch_metrics, safe_float, safe_int


def render_clustering(window_seconds: int) -> None:
    st.header("Clustering Metrics")
    df_clu = fetch_metrics("clustering", window_seconds)

    if df_clu.empty:
        st.info("No clustering metrics found.")
        return

    # Ensure columns exist and handle NaN values
    for col in ["silhouette_score", "dbcv_score", "num_clusters", "noise_ratio"]:
        if col not in df_clu.columns:
            df_clu[col] = 0.0
        else:
            # Fill NaN values with appropriate defaults
            if col == "num_clusters":
                df_clu[col] = df_clu[col].fillna(0).astype(float)
            else:
                df_clu[col] = df_clu[col].fillna(0.0)

    # Overview Charts
    st.subheader("Clustering Quality Over Time")
    st.line_chart(df_clu.set_index("timestamp")[["silhouette_score", "dbcv_score"]])

    # Latest Metrics
    st.subheader("Latest Run Details")
    latest = df_clu.iloc[0]
    col1, col2, col3, col4 = st.columns(4)
    col1.metric(
        "DBCV Score",
        f"{safe_float(latest.get('dbcv_score', 0.0)):.3f}",
        help="Density-Based Clustering Validation. Higher is better.",
    )
    col2.metric("Silhouette Score", f"{safe_float(latest.get('silhouette_score', 0.0)):.3f}")
    col3.metric("Num Clusters", safe_int(latest.get("num_clusters", 0)))
    col4.metric(
        "Noise Ratio",
        f"{safe_float(latest.get('noise_ratio', 0.0)):.2%}",
        help="Percentage of articles classified as noise (-1).",
    )

    # Cluster Size Distribution
    # Check if 'cluster_sizes' exists and is a list
    if "cluster_sizes" in latest and isinstance(latest["cluster_sizes"], list) and latest["cluster_sizes"]:
        st.subheader("Cluster Size Distribution (Latest)")
        sizes = latest["cluster_sizes"]

        # Simple histogram using bar chart of counts
        # Binning manually or using numpy
        try:
            counts, bin_edges = np.histogram(sizes, bins="auto")
            # Create a dataframe for the bar chart
            # We want labels like "4-6", "6-8"
            bin_labels = [f"{int(bin_edges[i])}-{int(bin_edges[i+1])}" for i in range(len(bin_edges) - 1)]
            hist_df = pd.DataFrame({"Count": counts}, index=bin_labels)
            st.bar_chart(hist_df)

            st.caption(f"Min Size: {min(sizes)}, Max Size: {max(sizes)}, Median: {np.median(sizes)}")
        except (ValueError, TypeError) as e:
            st.error(f"Error plotting cluster sizes: {e}")
            st.write(sizes)
    elif "cluster_sizes" not in latest:
        st.warning("Cluster size data ('cluster_sizes') is missing from logs. Update backend to log this list.")

    # Outlier Analysis over time
    st.subheader("Noise Ratio Trend")
    st.area_chart(df_clu.set_index("timestamp")["noise_ratio"])
