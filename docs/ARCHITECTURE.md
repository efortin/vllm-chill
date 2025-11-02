# vLLM AutoScaler - Architecture

## Pourquoi un proxy séparé et pas un sidecar ?

### ❌ Sidecar impossible pour scale-to-zero

Un **sidecar** partage le même pod que vLLM. Si on scale le deployment à 0 :
- Le pod entier est terminé
- Le sidecar autoscaler est aussi terminé
- Plus personne pour détecter les requêtes et wake vLLM
- **Scale-to-zero impossible**

### ✅ Proxy séparé : la bonne solution

Le proxy tourne dans son **propre deployment** :
- Reste actif même quand vLLM est à 0 replicas
- Détecte les requêtes entrantes
- Scale vLLM de 0 → 1 automatiquement
- Buffer les connexions pendant le wake-up
- Scale vLLM de 1 → 0 après inactivité

## Architecture actuelle

```
┌─────────────────────────────────────────────────────────────┐
│                         Internet                             │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Ingress (nginx)                           │
│                 vllm.sir-alfred.io                           │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              vllm-autoscaler-svc:80                          │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│         vllm-autoscaler (Deployment: 1 replica)              │
│                                                               │
│  • Toujours actif (ne scale jamais à 0)                     │
│  • Détecte si vLLM est à 0 replicas                         │
│  • Scale vLLM à 1 si nécessaire                             │
│  • Attend que vLLM soit Ready (max 2min)                    │
│  • Buffer les connexions pendant le wake                     │
│  • Proxy les requêtes vers vLLM                             │
│  • Track l'activité                                          │
│  • Scale vLLM à 0 après 5min idle                           │
│                                                               │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                   vllm-svc:80                                │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              vLLM (Deployment: 0 ou 1 replica)               │
│                                                               │
│  • Scale à 0 quand inactif → Libère les GPUs                │
│  • Scale à 1 sur requête → Charge le modèle                 │
│  • Startup: ~36 secondes                                     │
│  • Health probes avec Authorization                          │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## Avantages de cette architecture

### ✅ Scale-to-zero fonctionnel
- Le proxy reste actif pour détecter les requêtes
- vLLM peut être complètement arrêté (0 replicas)
- GPUs 100% libérés quand inactif

### ✅ Overhead minimal
- Proxy : ~50-80MB RAM, <5ms latency
- Coût négligeable vs économie GPU

### ✅ Séparation des responsabilités
- Proxy : Routing + scaling logic
- vLLM : Inference uniquement
- Chaque composant peut être mis à jour indépendamment

### ✅ Résilience
- Si vLLM crash, le proxy reste actif
- Peut redémarrer vLLM automatiquement
- Logs séparés pour debugging

## Alternatives considérées

### Option 1 : Sidecar ❌
**Problème** : Impossible de scale à 0 (le sidecar serait aussi terminé)

### Option 2 : KEDA HTTP Add-on ❌
**Problèmes** :
- Timeout hardcodé de 20s (vLLM = 60s)
- Complexité (Helm, CRDs, namespaces)
- Pas de contrôle fin

### Option 3 : CronJob ❌
**Problème** : Pas de wake automatique (wake manuel requis)

### Option 4 : Proxy séparé ✅
**Solution choisie** : Simple, fiable, scale-to-zero fonctionnel

## Ressources

### Proxy AutoScaler
- **CPU** : 100m request, 200m limit
- **RAM** : 64Mi request, 128Mi limit
- **Replicas** : 1 (toujours actif)

### vLLM
- **CPU** : Pas de limite (GPU workload)
- **RAM** : 16Gi request, 32Gi limit
- **GPU** : 2× RTX 3090 (nvidia.com/gpu: 2)
- **Replicas** : 0 ou 1 (dynamique)

## Métriques observées

- **Wake time** : ~36 secondes
- **Scale-down delay** : 5 minutes après dernière requête
- **Proxy overhead** : <5ms par requête
- **Économie GPU** : 100% quand inactif
- **Uptime proxy** : 100% (ne scale jamais)

## Conclusion

L'architecture avec **proxy séparé** est la seule solution viable pour :
1. Scale-to-zero fonctionnel
2. Wake automatique transparent
3. Overhead minimal
4. Simplicité et maintenabilité

Le sidecar est une bonne pratique dans beaucoup de cas, mais **pas pour le scale-to-zero**.
