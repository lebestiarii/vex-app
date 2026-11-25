import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { CardLayoutType } from '../models/card-node.model';
import { ActionListCardComponent } from '../templates/action-list-card.component';
import { ConvertCardComponent } from '../templates/convert-card.component';
import { FactsCardComponent } from '../templates/facts-card.component';
import { GroupCardComponent } from '../templates/group-card.component';
import { LinkCardComponent } from '../templates/link-card.component';
import { SegmentsCardComponent } from '../templates/segments-card.component';
import { SnippetCardComponent } from '../templates/snippet-card.component';
import { StepsCardComponent } from '../templates/steps-card.component';
import { TableCardComponent } from '../templates/table-card.component';
import { CardTemplateRegistry } from './card-template-registry.service';

describe('CardTemplateRegistry', () => {
  let registry: CardTemplateRegistry;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    registry = TestBed.inject(CardTemplateRegistry);
  });

  it('resolves all 9 pure generic layouts to their exact components', () => {
    expect(registry.resolve('group')).toBe(GroupCardComponent);
    expect(registry.resolve('link')).toBe(LinkCardComponent);
    expect(registry.resolve('action-list')).toBe(ActionListCardComponent);
    expect(registry.resolve('facts')).toBe(FactsCardComponent);
    expect(registry.resolve('segments')).toBe(SegmentsCardComponent);
    expect(registry.resolve('steps')).toBe(StepsCardComponent);
    expect(registry.resolve('convert')).toBe(ConvertCardComponent);
    expect(registry.resolve('table')).toBe(TableCardComponent);
    expect(registry.resolve('snippet')).toBe(SnippetCardComponent);
  });

  it('falls back defensively to FactsCardComponent for unknown layouts', () => {
    expect(registry.resolve('unknown-layout' as CardLayoutType)).toBe(FactsCardComponent);
  });
});
