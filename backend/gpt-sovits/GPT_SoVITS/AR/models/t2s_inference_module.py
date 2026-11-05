"""Inference-only semantic model container for the Whale speech bundle.

Preserves checkpoint key names (model.*) without importing a trainer,
optimizers, dataset loaders, callbacks, or training loggers.
"""
from torch import nn
from AR.models.t2s_model import Text2SemanticDecoder


class Text2SemanticInferenceModule(nn.Module):
    def __init__(self, config):
        super().__init__()
        self.config = config
        self.model = Text2SemanticDecoder(config=config, top_k=3)
