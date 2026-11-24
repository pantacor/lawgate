# shop laws

## no-silent-errors

No site of the `pay` machine swallows an error.

- Source: owner decision 2026-11-24
- Enforced by: machine `pay`
- Status: agreed 2026-11-24

## never-overspend

No payment is recorded over the balance.

- Source: owner decision 2026-11-24
- Enforced by: test `TestRefusesOverBalance`
- Subject: example.com/shop.affordable
- Status: agreed 2026-11-24
