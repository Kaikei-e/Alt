
import streamlit as st
from utils import fetch_metrics

def render_classification():
    st.header("Classification Metrics")
    df_cls = fetch_metrics("classification")
    if not df_cls.empty:
        # Ensure expected columns exist
        for col in ['accuracy', 'macro_f1', 'hamming_loss']:
            if col not in df_cls.columns:
                df_cls[col] = 0.0

        st.subheader("Accuracy Over Time")
        st.line_chart(df_cls, x="timestamp", y="accuracy")

        st.subheader("Latest Metrics")
        latest = df_cls.iloc[0]
        col1, col2, col3 = st.columns(3)
        col1.metric("Accuracy", f"{latest['accuracy']:.2%}")
        col2.metric("Macro F1", f"{latest['macro_f1']:.2f}")
        col3.metric("Hamming Loss", f"{latest['hamming_loss']:.4f}")

        if 'per_genre' in df_cls.columns:
            st.subheader("Per-Genre F1 Scores (Latest)")
            f1_cols = [c for c in df_cls.columns if "per_genre" in c and ".f1-score" in c]
            if f1_cols:
                latest_f1 = df_cls.iloc[0][f1_cols]
                latest_f1.index = [c.split('.')[1] for c in latest_f1.index]
                st.bar_chart(latest_f1)
            else:
                 st.info("No detailed per-genre metrics available.")
    else:
        st.info("No classification metrics found.")
