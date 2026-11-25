import { Injectable, Type } from '@angular/core';
import { CardLayoutType } from '../models/card-node.model';
import { ActionListCardComponent } from '../templates/action-list-card.component';
import { BaseCardTemplate } from '../templates/base-card-template';
import { ConvertCardComponent } from '../templates/convert-card.component';
import { FactsCardComponent } from '../templates/facts-card.component';
import { GroupCardComponent } from '../templates/group-card.component';
import { LinkCardComponent } from '../templates/link-card.component';
import { SegmentsCardComponent } from '../templates/segments-card.component';
import { SnippetCardComponent } from '../templates/snippet-card.component';
import { StepsCardComponent } from '../templates/steps-card.component';
import { TableCardComponent } from '../templates/table-card.component';

@Injectable({ providedIn: 'root' })
export class CardTemplateRegistry {
  private readonly registry = new Map<CardLayoutType, Type<BaseCardTemplate<any>>>([
    ['group', GroupCardComponent],
    ['link', LinkCardComponent],
    ['action-list', ActionListCardComponent],
    ['facts', FactsCardComponent],
    ['segments', SegmentsCardComponent],
    ['steps', StepsCardComponent],
    ['convert', ConvertCardComponent],
    ['table', TableCardComponent],
    ['snippet', SnippetCardComponent],
  ]);

  resolve(layout: CardLayoutType): Type<BaseCardTemplate<any>> {
    return this.registry.get(layout) ?? FactsCardComponent;
  }

  register(layout: CardLayoutType, component: Type<BaseCardTemplate<any>>): void {
    this.registry.set(layout, component);
  }
}
