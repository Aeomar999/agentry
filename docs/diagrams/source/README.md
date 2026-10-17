# The figures

`figures.py` draws every figure in `docs/diagrams`, with `flat.py`: the
svg-diagrams kit's layout and labelling, drawn flat to match the app.
Standard library Python only.

```sh
cd docs/diagrams/source && python3 figures.py ..
```

Render each one and look at it before committing (any headless Chromium:
`--screenshot` of the `.svg`); text widths are estimates.
