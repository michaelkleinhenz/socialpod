import { useEffect, useState } from 'react';
import { api } from '../../services/api';
import type { AgentModel } from '../../types';

let modelsPromise: Promise<AgentModel[]> | null = null;

/** The OpenRouter models that can call tools, fetched once per page load. */
export function useAgentModels(): AgentModel[] {
  const [models, setModels] = useState<AgentModel[]>([]);
  useEffect(() => {
    if (!modelsPromise) {
      modelsPromise = api.getAgentModels().catch(() => {
        modelsPromise = null;
        return [];
      });
    }
    let alive = true;
    modelsPromise.then(m => { if (alive) setModels(m); });
    return () => { alive = false; };
  }, []);
  return models;
}
