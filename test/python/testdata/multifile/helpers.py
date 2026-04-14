def helper(n):
    return n * 2


def compute(n):
    return helper(n) + 1


def also_calls_compute():
    return compute(7)
