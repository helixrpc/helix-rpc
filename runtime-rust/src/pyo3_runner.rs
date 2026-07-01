use pyo3::prelude::*;
use pyo3::types::{PyList, PyString};
use std::sync::Arc;

pub struct PyModelHandler {
    model: PyObject,
}

impl PyModelHandler {
    pub fn new(python_path: &str, module_name: &str, class_name: &str) -> PyResult<Self> {
        Python::with_gil(|py| -> PyResult<Self> {
            let sys = py.import("sys")?;
            let path = sys.getattr("path")?.downcast_into::<PyList>()?;
            path.insert(0, python_path)?;

            let module = py.import(module_name)?;
            let model_class = module.getattr(class_name)?;
            let model = model_class.call0()?.into();

            Ok(PyModelHandler { model })
        })
    }

    pub fn generate_batch(&self, prompts: Vec<String>) -> PyResult<Vec<String>> {
        Python::with_gil(|py| -> PyResult<Vec<String>> {
            let py_prompts = PyList::new(py, prompts)?;
            
            let result = self.model.call_method1(py, "generate_batch", (py_prompts,))?;
            
            let result_list = result.downcast_bound::<PyList>(py)?;
            
            let mut out = Vec::with_capacity(result_list.len());
            for item in result_list.iter() {
                let s = item.downcast::<PyString>()?;
                out.push(s.to_str()?.to_owned());
            }
            
            Ok(out)
        })
    }

    /// Iterates over a Python generator and pipes tokens to an MPSC channel.
    pub fn generate_stream(
        self: Arc<Self>,
        prompt: String,
        tx: tokio::sync::mpsc::Sender<Result<String, String>>,
    ) {
        tokio::task::spawn_blocking(move || {
            let res = Python::with_gil(|py| -> PyResult<()> {
                let py_prompt = pyo3::types::PyString::new(py, &prompt);
                
                let result = self.model.call_method1(py, "generate_stream", (py_prompt,))?;
                
                let iter = result.downcast_bound::<pyo3::types::PyIterator>(py)?;
                
                for item in iter {
                    match item {
                        Ok(it) => {
                            if let Ok(s) = it.downcast::<pyo3::types::PyString>() {
                                if let Ok(s_str) = s.to_str() {
                                    // Send to channel, ignoring send errors (e.g. client disconnected)
                                    let _ = tx.blocking_send(Ok(s_str.to_owned()));
                                }
                            }
                        }
                        Err(e) => {
                            let _ = tx.blocking_send(Err(e.to_string()));
                            break;
                        }
                    }
                }
                Ok(())
            });

            if let Err(e) = res {
                let _ = tx.blocking_send(Err(e.to_string()));
            }
        });
    }
}
