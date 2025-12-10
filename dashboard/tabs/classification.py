
import streamlit as st
import pandas as pd
from utils import fetch_metrics


def render_classification(window_seconds: int):
    st.header("Classification Metrics")
    df_cls = fetch_metrics("classification", window_seconds)

    if df_cls.empty:
        st.info("No classification metrics found.")
        return

    # Ensure expected top-level columns exist and handle NaN values
    for col in ['accuracy', 'macro_f1', 'hamming_loss']:
        if col not in df_cls.columns:
            df_cls[col] = 0.0
        else:
            # Fill NaN values with appropriate defaults
            df_cls[col] = df_cls[col].fillna(0.0)

    st.subheader("Overall Performance Over Time")
    col1, col2 = st.columns([3, 1])
    with col1:
        st.line_chart(df_cls.set_index("timestamp")[['accuracy', 'macro_f1']])
    with col2:
        latest = df_cls.iloc[0]
        accuracy = latest.get('accuracy', 0.0) if not pd.isna(latest.get('accuracy')) else 0.0
        macro_f1 = latest.get('macro_f1', 0.0) if not pd.isna(latest.get('macro_f1')) else 0.0
        hamming_loss = latest.get('hamming_loss', 0.0) if not pd.isna(latest.get('hamming_loss')) else 0.0
        st.metric("Latest Accuracy", f"{accuracy:.2%}")
        st.metric("Latest Macro F1", f"{macro_f1:.2f}")
        st.metric("Hamming Loss", f"{hamming_loss:.4f}")

    # Detailed Per-Genre Analysis
    st.subheader("Per-Genre Analysis (Latest)")

    # Identify genre keys from columns (assuming format per_genre.<genre>.<metric>)
    # We look for per_genre columns
    per_genre_cols = [c for c in df_cls.columns if c.startswith("per_genre.")]

    if per_genre_cols:
        # Extract genres and metrics
        # Example col: per_genre.sports.f1-score
        genres = set()
        for c in per_genre_cols:
            parts = c.split('.')
            if len(parts) >= 3:
                genres.add(parts[1])

        genres_list = sorted(list(genres))

        # Build a dataframe for the latest run
        genre_data = []
        latest_row = df_cls.iloc[0]

        for g in genres_list:
            row = {"Genre": g}
            # Try to find standard metrics, handling NaN values
            precision_val = latest_row.get(f"per_genre.{g}.precision", 0.0)
            row["Precision"] = 0.0 if pd.isna(precision_val) else precision_val
            recall_val = latest_row.get(f"per_genre.{g}.recall", 0.0)
            row["Recall"] = 0.0 if pd.isna(recall_val) else recall_val
            f1_val = latest_row.get(f"per_genre.{g}.f1-score", 0.0)
            row["F1"] = 0.0 if pd.isna(f1_val) else f1_val
            threshold_val = latest_row.get(f"per_genre.{g}.threshold", 0.5)
            row["Threshold"] = 0.5 if pd.isna(threshold_val) else threshold_val
            support_val = latest_row.get(f"per_genre.{g}.support", 0)
            row["Support"] = 0 if pd.isna(support_val) else int(support_val)
            genre_data.append(row)

        df_genes = pd.DataFrame(genre_data)

        if not df_genes.empty:
            st.dataframe(
                df_genes.style.background_gradient(subset=['Precision', 'Recall', 'F1'], cmap="viridis", vmin=0, vmax=1)
                        .format({"Precision": "{:.2%}", "Recall": "{:.2%}", "F1": "{:.2f}", "Threshold": "{:.2f}", "Support": "{:.0f}"}),
                use_container_width=True
            )

            # Visualization of Thresholds
            st.subheader("Dynamic Thresholds by Genre")
            st.bar_chart(df_genes.set_index("Genre")["Threshold"])

    else:
        st.info("No detailed per-genre metrics available in the latest log.")

    # Confusion Matrix Visualization (if available)
    # This requires 'confusion_matrix' in metrics which might be a complex object or flattened.
    # Usually confusion matrix is too large to flatten properly in simple tabular logs,
    # but if stored as a json string or blob, we might handle it.
    # For now, we skip unless we see a clear pattern.

