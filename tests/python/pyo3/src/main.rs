fn main() {
    println!("Initializing Zero-Serialization PyO3 Rust AI Runtime...");

    // The Python file `model.py` is located in the current directory
    let python_path = ".";
    let module_name = "model";
    let class_name = "DummyModel";

    // Initialize the wrapper. PyO3 auto-initializes the interpreter because of the feature flag.
    let handler = match helix_rt::pyo3_runner::PyModelHandler::new(python_path, module_name, class_name) {
        Ok(h) => h,
        Err(e) => {
            eprintln!("Failed to load python model: {}", e);
            std::process::exit(1);
        }
    };

    println!("Successfully embedded Python interpreter and instantiated DummyModel!");

    // Simulate the Go/Rust batch scheduler passing a vector of requests
    let prompts = vec![
        "What is the capital of France?".to_string(),
        "Translate 'hello' to Spanish.".to_string(),
        "Summarize the plot of Inception.".to_string(),
    ];

    println!("Sending batch of {} prompts for in-memory Zero-Copy inference...", prompts.len());

    // Execute the batch IN-PROCESS
    match handler.generate_batch(prompts) {
        Ok(responses) => {
            println!("\nReceived responses natively from Python:");
            for (i, resp) in responses.iter().enumerate() {
                println!("  Response {}: {}", i + 1, resp);
            }
        },
        Err(e) => {
            eprintln!("Failed to generate batch: {}", e);
            std::process::exit(1);
        }
    }
}
