<template>
  <div class="search-results">
    <!-- Search Results List (chunks merged per document) -->
    <div v-if="groupedResults.length > 0" class="results-list">
      <ResultRow
        v-for="(group, idx) in groupedResults"
        :key="group.key"
        :index="idx + 1"
        :title="group.title"
        :meta="$t('agentStream.grepResults.chunkHits', { count: group.chunks.length })"
        :popup-key="group.knowledge_id"
        :chunks="group.chunks"
        :chunk-id="group.chunks.length === 1 ? group.chunks[0].chunk_id : undefined"
        :knowledge-id="group.knowledge_id"
        :highlight="highlightQuery"
        :provenance-inputs="provenanceInputsFor(group)"
        :provenance-title="group.title"
      />
    </div>

    <!-- Empty State -->
    <div v-else class="empty-state">
      {{ $t('chat.noSearchResults') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SearchResultsData, SearchResultItem, RelevanceLevel } from '@/types/tool-results';
import type { ProvenanceInput } from '@/utils/provenance';
import { getMatchTypeIcon } from '@/utils/tool-icons';
import ResultRow from './ResultRow.vue';
import { useI18n } from 'vue-i18n';

const props = defineProps<{
  data: SearchResultsData;
  arguments?: Record<string, any> | string;
}>();

const { t } = useI18n();

const results = computed(() => props.data.results || []);
const kbCounts = computed(() => props.data.kb_counts);

interface GroupedChunk {
  content: string;
  chunk_id: string;
  knowledge_id: string;
  knowledge_title?: string;
  knowledge_base_id?: string;
  metadata?: Record<string, string>;
  chunk_metadata?: Record<string, unknown>;
}

interface GroupedResult {
  key: string;
  knowledge_id: string;
  title: string;
  chunks: GroupedChunk[];
}

// 溯源面板取数：把分组后的原始 chunk 行还原成 ProvenanceInput。
// 工具结果行当前不携带 chunk_metadata，面板会渲染“未随下发”空态；
// 后端一旦透传即可直接点亮，无需再改这里。
function provenanceInputsFor(group: GroupedResult): ProvenanceInput[] {
  return group.chunks.map((chunk) => ({
    id: chunk.chunk_id,
    content: chunk.content,
    knowledge_id: chunk.knowledge_id || group.knowledge_id,
    knowledge_title: chunk.knowledge_title || group.title,
    knowledge_base_id: chunk.knowledge_base_id,
    metadata: chunk.metadata,
    chunk_metadata: chunk.chunk_metadata,
  }));
}

// Hybrid retrieval can return several chunks from the same document; collapse
// them into one row so the same file is not listed repeatedly. FAQ entries are
// the exception: they all share the owning document's title, so each entry is
// kept as its own row labelled by its standard question to stay distinguishable.
const groupedResults = computed<GroupedResult[]>(() => {
  const map = new Map<string, GroupedResult>();
  const order: string[] = [];
  for (const r of results.value) {
    const faqQuestion = r.faq_standard_question?.trim();
    const isFaq = !!faqQuestion;
    const key = isFaq ? r.chunk_id : r.knowledge_id || r.chunk_id;
    if (!map.has(key)) {
      map.set(key, {
        key,
        knowledge_id: r.knowledge_id,
        title: (isFaq ? faqQuestion : r.knowledge_title) || r.knowledge_title,
        chunks: [],
      });
      order.push(key);
    }
    map.get(key)!.chunks.push({
      content: r.content,
      chunk_id: r.chunk_id,
      knowledge_id: r.knowledge_id,
      knowledge_title: r.knowledge_title,
      knowledge_base_id: r.knowledge_base_id,
      metadata: r.metadata,
      chunk_metadata: r.chunk_metadata,
    });
  }
  return order.map((k) => map.get(k)!);
});

// Parse arguments if it's a string
const parsedArguments = computed(() => {
  const args = props.arguments;
  if (!args) return null;
  
  // If it's already an object, return it
  if (typeof args === 'object' && !Array.isArray(args)) {
    return args;
  }
  
  // If it's a string, try to parse it
  if (typeof args === 'string') {
    try {
      return JSON.parse(args);
    } catch (e) {
      console.warn('Failed to parse arguments:', e);
      return null;
    }
  }
  
  return null;
});

// Highlight terms for the content popup: the natural-language query plus any
// explicit sub-queries passed to the search tool.
const highlightQuery = computed(() => {
  const parts: string[] = [];
  const args = parsedArguments.value;
  if (args && typeof args === 'object') {
    if (typeof args.query === 'string') parts.push(args.query);
    if (Array.isArray(args.queries)) {
      parts.push(...args.queries.filter((q: unknown): q is string => typeof q === 'string'));
    }
  }
  if (parts.length === 0 && props.data.query) parts.push(props.data.query);
  return parts.join(' ');
});

// Check if there are search parameters to display (excluding query parameters which are in title)
const hasSearchParams = computed(() => {
  const args = parsedArguments.value;
  if (!args || typeof args !== 'object') return false;
  
  return !!(
    (Array.isArray(args.knowledge_base_ids) && args.knowledge_base_ids.length > 0) ||
    args.top_k || args.vector_threshold || args.keyword_threshold || args.min_score);
});

const hasOtherParams = computed(() => {
  const args = parsedArguments.value;
  if (!args || typeof args !== 'object') return false;
  return !!(args.top_k || args.vector_threshold || args.keyword_threshold || args.min_score);
});


const getRelevanceClass = (level: RelevanceLevel): string => {
  const classMap: Record<RelevanceLevel, string> = {
    'High Relevance': 'high',
    'Medium Relevance': 'medium',
    'Low Relevance': 'low',
    'Weak Relevance': 'weak',
  };
  return classMap[level] || 'weak';
};

const getRelevanceLabel = (level: RelevanceLevel): string => {
  const labelMap: Record<RelevanceLevel, string> = {
    'High Relevance': t('chat.relevanceHigh'),
    'Medium Relevance': t('chat.relevanceMedium'),
    'Low Relevance': t('chat.relevanceLow'),
    'Weak Relevance': t('chat.relevanceWeak'),
  };
  return labelMap[level] || level;
};
</script>

<style lang="less" scoped>
@import './tool-results.less';

.search-results {
  display: flex;
  flex-direction: column;
  padding: 0 0 0 12px;
  gap: 3px;
}

.results-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
  max-height: 200px;
  overflow-y: auto;
}
</style>


