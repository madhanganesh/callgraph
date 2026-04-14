fn helper(n: i32) -> i32 {
    n * 2
}

pub fn compute(n: i32) -> i32 {
    helper(n) + 1
}

pub fn also_calls_compute() -> i32 {
    compute(7)
}
