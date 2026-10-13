import { useId } from 'react';
import { useAgentModels } from './useAgentModels';

const price = (n: number) => (n === 0 ? 'free' : `$${n < 1 ? n.toFixed(2) : n.toFixed(n < 10 ? 2 : 0)}`);

interface ModelPickerProps {
  value: string;
  onChange: (id: string) => void;
  placeholder?: string;
}

/**
 * A text field with OpenRouter's tool-calling models as suggestions. Any
 * model id can be typed; the line below says what the chosen one costs.
 */
export function ModelPicker({ value, onChange, placeholder }: ModelPickerProps) {
  const models = useAgentModels();
  const listId = useId();
  const selected = models.find(m => m.id === value.trim());

  return (
    <>
      <input
        className="input"
        list={listId}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder ?? 'e.g. anthropic/claude-sonnet-4.5'}
        spellCheck={false}
        autoComplete="off"
      />
      <datalist id={listId}>
        {models.map(m => (
          <option key={m.id} value={m.id}>
            {m.name} · {price(m.promptPrice)} in / {price(m.completionPrice)} out per 1M tokens
          </option>
        ))}
      </datalist>
      {value.trim() && (
        <span className="agent-hint">
          {selected
            ? `${selected.name}: ${price(selected.promptPrice)} input, ${price(selected.completionPrice)} output per million tokens, ` +
              `${Math.round(selected.contextLength / 1000)}k context${selected.images ? ', sees images' : ', text only'}.`
            : models.length > 0
              ? 'Not in OpenRouter\'s list of models that can call tools; agents need tool calling.'
              : ''}
        </span>
      )}
    </>
  );
}
