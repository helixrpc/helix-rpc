try:
    import numpy as np
    _NUMPY_AVAILABLE = True
except ImportError:
    _NUMPY_AVAILABLE = False
    np = None  # type: ignore[assignment]


def _require_numpy():
    if not _NUMPY_AVAILABLE:
        raise ImportError(
            "numpy is required for Tensor operations. "
            "Install it with: pip install numpy"
        )


class Tensor:
    def __init__(self, dtype: str, shape: list, data: bytes):
        self.dtype = dtype
        self.shape = list(shape)
        self.data = data

    def to_numpy(self):
        """Returns a zero-copy numpy array pointing directly to the underlying data buffer."""
        _require_numpy()
        np_type = self._resolve_np_type()
        # Create zero-copy view from bytes buffer
        arr = np.frombuffer(self.data, dtype=np_type)
        return arr.reshape(self.shape)

    @classmethod
    def from_numpy(cls, arr) -> "Tensor":
        """Constructs a Tensor from a numpy array without duplicating array bytes if contiguous."""
        _require_numpy()
        if not arr.flags["C_CONTIGUOUS"]:
            arr = np.ascontiguousarray(arr)

        dtype_str = str(arr.dtype)
        return cls(
            dtype=dtype_str,
            shape=list(arr.shape),
            data=arr.tobytes()
        )

    def _resolve_np_type(self):
        _require_numpy()
        mapping = {
            "float32": np.float32,
            "float64": np.float64,
            "float16": np.float16,
            "int32": np.int32,
            "int64": np.int64,
            "int8": np.int8,
            "uint8": np.uint8,
        }
        t = mapping.get(self.dtype)
        if t is None:
            if "float32" in self.dtype:
                return np.float32
            if "int64" in self.dtype:
                return np.int64
            return np.float32
        return t
