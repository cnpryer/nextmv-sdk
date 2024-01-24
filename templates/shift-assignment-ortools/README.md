# Shift assignment with OR-Tools

This app solves a shift asssignment problem using [OR-Tools][or-tools]. Given a
set of previously planned shifts, in this app we assign workers to those shifts,
taking different factors into account such as availability and qualification.

The most important files created are `main.py` and `input.json`.

* `main.py` implements a MIP knapsack solver.
* `input.json` is a sample input file.

## Usage

Follow these steps to run locally.

1. Make sure that all the required packages are installed:

    ```bash
    pip3 install -r requirements.txt
    ```

1. Run the command below to check that everything works as expected:

    ```bash
    python3 main.py -input input.json -output output.json -duration 30
    ```

1. A file `output.json` should have been created with a solution to the shift assignment problem.

[or-tools]: https://developers.google.com/optimization
