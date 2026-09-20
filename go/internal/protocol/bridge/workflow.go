package bridge

// Accepted only by Clauduct's Workflow adapter; native receives a generated,
// reviewable script containing the validated plan, never this marker.
const WorkflowPlanMarker = "clauduct:plan-v1"

// A refused operation is replaced before delivery. Even if native skips or loses
// the denying hook, this fixed script cannot execute the original task.
const RejectedWorkflowScript = "export const meta = {name:'clauduct-rejected-recovery',description:'Rejected recovery; no task is executed'};\nthrow new Error('WORKFLOW_RECOVERY_UNVERIFIED');"
const RejectedWorkflowReason = "WORKFLOW_RECOVERY_UNVERIFIED: this Workflow operation could not be verified and nothing was executed. Report the limitation or unavailable result; do not automatically retry or recreate the workflow. Continue with the next user request."
