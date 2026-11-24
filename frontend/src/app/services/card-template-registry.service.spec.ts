import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { CardTemplateType } from '../models/card-node.model';
import { BranchContactCardComponent } from '../templates/branch-contact-card.component';
import { ContainerBlueprintCardComponent } from '../templates/container-blueprint-card.component';
import { ConversionCardComponent } from '../templates/conversion-card.component';
import { DictionaryGuideCardComponent } from '../templates/dictionary-guide-card.component';
import { GenericKvCardComponent } from '../templates/generic-kv-card.component';
import { LocationCardComponent } from '../templates/location-card.component';
import { PatternBreakdownCardComponent } from '../templates/pattern-breakdown-card.component';
import { ServerMatrixCardComponent } from '../templates/server-matrix-card.component';
import { WebsiteLinkCardComponent } from '../templates/website-link-card.component';
import { CardTemplateRegistry } from './card-template-registry.service';

describe('CardTemplateRegistry', () => {
  let registry: CardTemplateRegistry;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    registry = TestBed.inject(CardTemplateRegistry);
  });

  it('resolves all 5 canonical generic templates', () => {
    expect(registry.resolve('location')).toBe(LocationCardComponent);
    expect(registry.resolve('server-matrix')).toBe(ServerMatrixCardComponent);
    expect(registry.resolve('website-link')).toBe(WebsiteLinkCardComponent);
    expect(registry.resolve('pattern-breakdown')).toBe(PatternBreakdownCardComponent);
    expect(registry.resolve('conversion')).toBe(ConversionCardComponent);
  });

  it('resolves backwards-compatible template aliases', () => {
    expect(registry.resolve('branch-gateway')).toBe(LocationCardComponent);
    expect(registry.resolve('env-bookmarks')).toBe(WebsiteLinkCardComponent);
    expect(registry.resolve('reference-decoder')).toBe(PatternBreakdownCardComponent);
  });

  it('resolves extended domain templates and falls back to GenericKvCardComponent', () => {
    expect(registry.resolve('container-blueprint')).toBe(ContainerBlueprintCardComponent);
    expect(registry.resolve('branch-contact')).toBe(BranchContactCardComponent);
    expect(registry.resolve('dictionary-guide')).toBe(DictionaryGuideCardComponent);
    expect(registry.resolve('kv-grid')).toBe(GenericKvCardComponent);
    expect(registry.resolve('unknown-template' as CardTemplateType)).toBe(GenericKvCardComponent);
  });
});
