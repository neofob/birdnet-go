from fastapi import FastAPI
from pydantic import BaseModel
import fasttext
import numpy as np

# fasttext (python) <-> NumPy 2.0 compatibility:
# Older fasttext releases call `np.array(probs, copy=False)` which raises
# in NumPy 2.0 when a copy is required. Use np.asarray instead.
from fasttext.FastText import _FastText  # type: ignore


def _predict_numpy2_compat(
    self: _FastText,
    text,
    k: int = 1,
    threshold: float = 0.0,
    on_unicode_error: str = "strict",
):
    def check(entry: str) -> str:
        if "\n" in entry:
            raise ValueError("predict processes one line at a time (remove '\\n')")
        return entry + "\n"

    if type(text) == list:
        text = [check(entry) for entry in text]
        all_labels, all_probs = self.f.multilinePredict(text, k, threshold, on_unicode_error)
        return all_labels, all_probs

    text = check(text)
    predictions = self.f.predict(text, k, threshold, on_unicode_error)
    if predictions:
        probs, labels = zip(*predictions)
    else:
        probs, labels = ([], ())

    # np.asarray allows a copy when needed (NumPy 1.x behavior).
    return labels, np.asarray(probs)


# Monkeypatch once at import time (applies to load_model() instances)
_FastText.predict = _predict_numpy2_compat

app = FastAPI()
model = fasttext.load_model("lid.176.ftz")
class Request(BaseModel):
    text: str
class Prediction(BaseModel):
    label: str
    confidence: float
class Response(BaseModel):
    label: str
    confidence: float
    top_n: list[Prediction]
@app.post("/classify")
def classify(req: Request):
    labels, probs = model.predict(req.text, k=5)
    top_n = [
        Prediction(label=l.replace("__label__", ""), confidence=float(p))
        for l, p in zip(labels, probs)
    ]
    return Response(
        label=top_n[0].label,
        confidence=top_n[0].confidence,
        top_n=top_n,
    )
