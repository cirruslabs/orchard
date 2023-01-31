load("github.com/cirrus-modules/golang@main", "lint_task")

def main(ctx):
    return [lint_task()]
