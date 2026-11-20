from typing import Self

import torch
from torch import nn
from transformers import AutoModelForSequenceClassification, AutoTokenizer
from transformers.modeling_outputs import SequenceClassifierOutput


class StudentDistilBERT(nn.Module):
    def __init__(self, model_name: str, num_labels: int) -> None:
        super().__init__()
        self.model_name = model_name
        self.num_labels = num_labels

        # 'line-corporation/line-distilbert-base-japanese' is DistilBERT
        self.tokenizer = AutoTokenizer.from_pretrained(model_name, trust_remote_code=False)
        self.bert = AutoModelForSequenceClassification.from_pretrained(
            model_name,
            num_labels=num_labels,
            problem_type="single_label_classification",
            trust_remote_code=False,
        )

    def forward(
        self,
        input_ids: torch.Tensor,
        attention_mask: torch.Tensor,
        labels: torch.Tensor | None = None,
    ) -> SequenceClassifierOutput:
        return self.bert(input_ids=input_ids, attention_mask=attention_mask, labels=labels)

    def save_pretrained(self, save_directory: str) -> None:
        self.bert.save_pretrained(save_directory)
        self.tokenizer.save_pretrained(save_directory)

    @classmethod
    def from_pretrained(cls, load_directory: str, num_labels: int = 30) -> Self:
        return cls(load_directory, num_labels)

    def predict(
        self, texts: list[str], max_length: int = 256, device: str | torch.device = "cpu"
    ) -> tuple[torch.Tensor, torch.Tensor]:
        self.bert.eval()
        inputs = self.tokenizer(
            texts, return_tensors="pt", padding=True, truncation=True, max_length=max_length
        ).to(device)

        with torch.no_grad():
            outputs = self.bert(**inputs)
            logits = outputs.logits
            probs = torch.softmax(logits, dim=-1)

        return probs, logits
