# Fixed-cost regression fixture

Eight common AWS resources with fixed (usage-independent) costs. Used
as a regression fixture: the project total must stay at **$292.99/mo**
to the cent against `pricing.c3x.dev`.

| Build | Pricing source | Total |
|---|---|---:|
| **c3x** | pricing.c3x.dev | **$292.99/mo** |

Re-run with:

```bash
c3x estimate --path examples/parity
```
