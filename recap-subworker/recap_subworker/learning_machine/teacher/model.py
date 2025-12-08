import torch
import torch.nn as nn
from transformers import AutoModelForSequenceClassification, AutoTokenizer
from typing import List, Dict, Any, Optional

class TeacherBERT(nn.Module):
    def __init__(self, model_name: str, num_labels: int, label_map: Dict[str, int] = None):
        super().__init__()
        self.model_name = model_name
        self.num_labels = num_labels
        self.label_map = label_map

        # Load pre-trained model and tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(model_name)
        self.bert = AutoModelForSequenceClassification.from_pretrained(
            model_name,
            num_labels=num_labels,
            problem_type="single_label_classification" # Or "multi_label_classification"
        )

    def forward(self, input_ids, attention_mask, labels=None):
        return self.bert(input_ids=input_ids, attention_mask=attention_mask, labels=labels)

    def save_pretrained(self, save_directory: str):
        self.bert.save_pretrained(save_directory)
        self.tokenizer.save_pretrained(save_directory)

    @classmethod
    def from_pretrained(cls, load_directory: str, num_labels: int = 30):
        # We assume label_map needs to be passed or loaded separately if needed logic relies on it
        instance = cls(load_directory, num_labels)
        return instance

    def predict(self, texts: List[str], max_length: int = 256, device="cpu"):
        self.eval()
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
