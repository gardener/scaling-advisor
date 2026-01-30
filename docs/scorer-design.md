## Node Scoring Approach

This document describes the current **Node Scoring** methodology used to select the most cost‑efficient **NodePlacement** for scale‑out, along with examples and known drawbacks.

---

Pods may request **multiple resource types** (CPU, memory, etc.). To compare heterogeneous resources uniformly, all requests are normalized into a single scalar unit called the **Normalized Resource Unit (NRU)**.

The scaling-advisor evaluates each candidate **NodePlacement** (instance type + availability zone) to generate a scale-out plan for a set of unscheduled pods.


## Normalized Resource Unit (NRU)

Each resource type is assigned a configurable **weight**. A pod’s request for each resource is multiplied by its weight to compute its NRU contribution.

### Example: CPU and Memory

Let:

* **CPU weight** = `x`
* **Memory weight** = `y`

Then:

* **CPU NRUs**
  `x × CPU cores requested`

* **Memory NRUs**
  `y × Memory (Gi) requested`

* **Total NRUs required by the pod**
  `(x × CPU) + (y × Memory)`

> **Current State**
> CPU and memory weights are currently static. Work is in progress to compute **instance‑family‑specific weights**, which will replace the static values.

---

## Node Score Calculation

For each NodePlacement, the **node score** is computed as:

```
node score = total NRUs scheduled / instance price
```

A **higher node score** indicates a **more desirable node** for scale‑out — i.e., more schedulable work per unit cost.

### “Total NRUs Scheduled”

This includes:

* NRUs required by pods scheduled on the **newly scaled‑out node**, and
* NRUs required by pods scheduled on **other nodes as a side effect of the scale‑up**.

> **Why this matters:**
> With **Topology Spread Constraints (TSCs)**, scaling up may trigger pod placements across *multiple* nodes, not just the newly added one. These secondary placements are included in the score.


## Node Score Selection Rules

1. Compute the node score for each candidate NodePlacement.
2. Select the NodePlacement with the **highest node score**.
3. If multiple NodePlacements have the same score, choose the one with the **larger capacity**, measured in NRUs.


## Weight Calibration

In evaluations so far:

* **CPU weight (x)** = `6`
* **Memory weight (y)** = `1`

These values were derived by solving multiple systems of linear equations and averaging the resulting **CPU‑to‑memory cost ratios** across instance types.

---

## Examples

### Scenario 1: Different Capacities and Costs

**Pod requirement**:

* 1 CPU core
* 2 Gi memory

```
NRUs = (1 × 6) + (2 × 1) = 8
```

**NodePlacements**:

| NodePlacement | Instance Type | Price | Capacity     | Score            |
| ------------- | ------------- |-------| ------------ |------------------|
| NP1           | m5.2xlarge    | $80   | 8 CPU, 32 Gi | 8 / 80 = 0.1     |
| NP2           | m5.xlarge     | $40   | 4 CPU, 16 Gi | 8 / 40 = 0.2 ✅ |

**Winner**: **NP2**, due to a higher NRU‑per‑dollar score.

---

### Scenario 2: Proportional Capacity and Cost

**Pod requirement**:

* 3 CPU cores
* 10 Gi memory

```
NRUs per replica = (3 × 6) + (10 × 1) = 28
2 replicas = 56 NRUs
```

**NodePlacements**:

| NodePlacement | Instance Type | Price | Capacity     | Score             |
| ------------- | ------------- |-------| ------------ |-------------------|
| NP1           | n2-standard-4 | $45   | 4 CPU, 16 Gi | 28 / 45 = 0.622   |
| NP2           | n2-standard-8 | $90   | 8 CPU, 32 Gi | 56 / 90 = 0.622 ✅ |

**Result**:

* Scores are effectively equal.
* **NP2** is selected due to its **larger NRU capacity**.

---

### Scenario 3: Same Capacity, Different Costs

**Pod requirement**:

* 1 CPU core
* 2 Gi memory

```
NRUs = 8
```

**NodePlacements**:

| NodePlacement | Instance Type | Price | Capacity     | Score  |
| ------------- | ------------- |-------| ------------ |--------|
| NP1           | m5.2xlarge    | $80   | 8 CPU, 32 Gi | 0.1    |
| NP2           | t4g.2xlarge   | $50   | 8 CPU, 32 Gi | 0.16 ✅ |

**Winner**: **NP2**, due to lower cost with identical capacity.

---

## Known Drawbacks

* The approach is **greedy**.
* Decisions are optimized for a **subset of currently unscheduled pods**.
* This can lead to **globally sub‑optimal scale‑ups**, especially when future pod placements or topology constraints are not considered.


### Scenario 4: Greedy Choice Leads to Higher Total Cost (Condensed)

**Pod requirements**: 3 replicas,
* 2 CPU core
* 8 Gi memory

**First Run:**


| NodePlacement | Instance Type | Price   | Capacity     | Score    |
| ------------- | ------------- | ------- | ------------ | -------- |
| NP1           | m.large       | $72     | 4 CPU, 16 Gi | 0.50 ✅  |
| NP2           | m.large       | $120    | 8 CPU, 32 Gi | 0.45     |


**Second run (1 pod remaining):**


| NodePlacement | Instance Type | Price   | Capacity     | Score    |
| ------------- | ------------- | ------- | ------------ | -------- |
| NP1           | m.large       | $72     | 4 CPU, 16 Gi | 0.25 ✅  |
| NP2           | m.large       | $120    | 8 CPU, 32 Gi | 0.15     |

**Result**:

Scaled nodes: 2 × NP1

Total cost: $144

**Optimal alternative:**
Selecting 1 × NP2 initially fits all pods

Total cost: $120

Takeaway: A greedy, step-by-step NRU-per-dollar decision can increase total cost even when a cheaper global solution exists.


### Other Scoring Formulas Considered

- **Initial approach** used `max(waste) + unscheduledRatio * costRatio`; multiplication caused **cost to be ignored** when no pods were unscheduled and could favor **smaller but more expensive instances**.
- **Subsequent variants** replaced `*` with `+`, but using `max(cpuWaste, memWaste)` masked meaningful differences when one resource (e.g., memory) dominated across instance types.
- These experiments reinforced that **waste should not penalize cheaper capacity** if more resources can be obtained for less cost.
- They also exposed that `unscheduledRatio` implicitly assumes **homogeneous pod resource demands**, which is invalid in practice.
- Finally, **directly adding quantities of different resource types** (CPU, memory) without normalization or economic grounding proved to be **semantically meaningless**.
