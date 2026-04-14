fn helper(n: i32) -> i32 {
    n * 2
}

fn compute(n: i32) -> i32 {
    helper(n) + 1
}

fn process() -> i32 {
    compute(42)
}

fn shortcut() -> i32 {
    compute(7) + 100
}

fn main() {
    println!("{} {}", process(), shortcut());
}
