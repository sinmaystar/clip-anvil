import { useState } from "react";
import type { AgentDecisionCardBlock as AgentDecisionCardBlockData } from "../../lib/agentMessageBlocks";

export interface AgentDecisionActions {
  disabled: boolean;
  resolvedDecisionIds: Set<string>;
  onSelectDecision: (decisionID: string, optionID: string) => void;
  onSubmitDecisionText: (decisionID: string, freeText: string) => void;
}

export function AgentDecisionCardBlock({
  block,
  actions,
}: {
  block: AgentDecisionCardBlockData;
  actions?: AgentDecisionActions;
}) {
  const [freeText, setFreeText] = useState("");
  const resolved =
    block.status === "handled" ||
    block.status === "cancelled" ||
    block.status === "failed" ||
    actions?.resolvedDecisionIds.has(block.decision_id) === true;
  const disabled = resolved || actions?.disabled === true;

  return (
    <div className="agent-decision-card">
      <div className="agent-decision-card-header">
        <strong>{block.title}</strong>
        <span>{resolved ? "已完成" : "待选择"}</span>
      </div>
      {block.message ? <p>{block.message}</p> : null}
      {block.options.length > 0 ? (
        <div className="agent-decision-options">
          {block.options.map((option) => (
            <button
              disabled={disabled || !actions}
              key={option.id}
              onClick={() => actions?.onSelectDecision(block.decision_id, option.id)}
              title={option.description}
              type="button"
            >
              {option.label}
            </button>
          ))}
        </div>
      ) : null}
      {block.allow_free_text && !resolved ? (
        <div className="agent-decision-free-text">
          <input
            aria-label="补充选择"
            disabled={disabled}
            onChange={(event) => setFreeText(event.target.value)}
            placeholder="补充说明"
            value={freeText}
          />
          <button
            disabled={disabled || freeText.trim() === "" || !actions}
            onClick={() =>
              actions?.onSubmitDecisionText(block.decision_id, freeText.trim())
            }
            type="button"
          >
            提交
          </button>
        </div>
      ) : null}
    </div>
  );
}
