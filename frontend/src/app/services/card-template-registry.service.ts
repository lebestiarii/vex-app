import { Injectable, Type } from '@angular/core';
import { CardTemplateType } from '../models/card-node.model';
import { BaseCardTemplate } from '../templates/base-card-template';
import { BranchContactCardComponent } from '../templates/branch-contact-card.component';
import { ContainerBlueprintCardComponent } from '../templates/container-blueprint-card.component';
import { ConversionCardComponent } from '../templates/conversion-card.component';
import { DictionaryGuideCardComponent } from '../templates/dictionary-guide-card.component';
import { GenericKvCardComponent } from '../templates/generic-kv-card.component';
import { LocationCardComponent } from '../templates/location-card.component';
import { PatternBreakdownCardComponent } from '../templates/pattern-breakdown-card.component';
import { ServerMatrixCardComponent } from '../templates/server-matrix-card.component';
import { WebsiteLinkCardComponent } from '../templates/website-link-card.component';

@Injectable({ providedIn: 'root' })
export class CardTemplateRegistry {
  private readonly registry = new Map<CardTemplateType, Type<BaseCardTemplate<any>>>([
    // 5 Canonical Generic Full-Width Stacked Templates
    ['location', LocationCardComponent],
    ['server-matrix', ServerMatrixCardComponent],
    ['website-link', WebsiteLinkCardComponent],
    ['pattern-breakdown', PatternBreakdownCardComponent],
    ['conversion', ConversionCardComponent],

    // Aliases & Extended Templates
    ['branch-gateway', LocationCardComponent],
    ['env-bookmarks', WebsiteLinkCardComponent],
    ['reference-decoder', PatternBreakdownCardComponent],
    ['container-blueprint', ContainerBlueprintCardComponent],
    ['branch-contact', BranchContactCardComponent],
    ['dictionary-guide', DictionaryGuideCardComponent],
    ['kv-grid', GenericKvCardComponent],
    ['shortcut', GenericKvCardComponent],
    ['knowledge', GenericKvCardComponent],
    ['command', GenericKvCardComponent],
    ['metric-card', GenericKvCardComponent],
  ]);

  resolve(template: CardTemplateType): Type<BaseCardTemplate<any>> {
    return this.registry.get(template) ?? GenericKvCardComponent;
  }

  register(template: CardTemplateType, component: Type<BaseCardTemplate<any>>): void {
    this.registry.set(template, component);
  }
}
