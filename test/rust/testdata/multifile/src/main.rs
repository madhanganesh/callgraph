mod helpers;

fn process() -> i32 {
    helpers::compute(42)
}

fn main() {
    println!("{} {}", process(), helpers::also_calls_compute());
}
