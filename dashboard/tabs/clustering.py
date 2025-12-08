
import streamlit as st
from utils import fetch_metrics

def render_clustering():
    st.header("Clustering Metrics")
    df_clu = fetch_metrics("clustering")
    if not df_clu.empty:
        for col in ['silhouette_score', 'dbcv_score']:
             if col not in df_clu.columns:
                 df_clu[col] = 0.0

        st.subheader("Clustering Quality")
        chart_data = df_clu[['timestamp', 'silhouette_score', 'dbcv_score']].copy()
        chart_data = chart_data.set_index('timestamp')
        st.line_chart(chart_data)

        col1, col2 = st.columns(2)
        latest = df_clu.iloc[0]
        col1.metric("Silhouette Score", f"{latest['silhouette_score']:.3f}")
        col2.metric("DBCV Score", f"{latest['dbcv_score']:.3f}")

    else:
        st.info("No clustering metrics found.")
