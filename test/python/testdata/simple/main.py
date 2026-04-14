def helper(n):
    return n * 2


def compute(n):
    return helper(n) + 1


def process():
    return compute(42)


def shortcut():
    return compute(7) + 100


def main():
    print(process(), shortcut())


if __name__ == "__main__":
    main()
