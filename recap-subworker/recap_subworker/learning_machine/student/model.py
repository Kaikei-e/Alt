import torch
import torch.nn as nn
from transformers import AutoModelForSequenceClassification, AutoTokenizer
from typing import List, Dict

class StudentDistilBERT(nn.Module):
    def __init__(self, model_name: str, num_labels: int):
        super().__init__()
        self.model_name = model_name
        self.num_labels = num_labels

        # 'line-corporation/line-distilbert-base-japanese' is DistilBERT
        self.tokenizer = AutoTokenizer.from_pretrained(model_name, trust_remote_code=True)
        self.bert = AutoModelForSequenceClassification.from_pretrained(
            model_name,
            num_labels=num_labels,
            problem_type="single_label_classification",
            trust_remote_code=True
        )

    def forward(self, input_ids, attention_mask, labels=None):
        return self.bert(input_ids=input_ids, attention_mask=attention_mask, labels=labels)

    def save_pretrained(self, save_directory: str):
        self.bert.save_pretrained(save_directory)
        self.tokenizer.save_pretrained(save_directory)

    @classmethod
    def from_pretrained(cls, load_directory: str, num_labels: int = 30):
        instance = cls(load_directory, num_labels)
        return instance

    def predict(self, texts: List[str], max_length: int = 256, device="cpu"):
        self.bert.eval()
        inputs = self.tokenizer(
            texts,
            return_tensors="pt",
            padding=True,
            truncation=True,
            max_length=max_length
        ).to(device)

        with torch.no_grad():
            outputs = self.bert(**inputs)
            logits = outputs.logits
            probs = torch.softmax(logits, dim=-1)

        return probs, logits
