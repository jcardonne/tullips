import { test } from "node:test";
import assert from "node:assert/strict";
import { removeWorkflowStep } from "../src/lib/api";
test("deleting a workflow step preserves branch destinations and clears deleted destinations", () => {
  const steps = [
    { type: "condition", then_step: 2, else_step: 3 },
    { type: "delay" },
    { type: "email" },
    { type: "stop" },
  ];
  assert.deepEqual(removeWorkflowStep(steps, 1), [
    { type: "condition", then_step: 1, else_step: 2 },
    { type: "email" },
    { type: "stop" },
  ]);
  assert.deepEqual(removeWorkflowStep(steps, 2), [
    { type: "condition", else_step: 2 },
    { type: "delay" },
    { type: "stop" },
  ]);
  assert.equal(steps[0].then_step, 2);
});
