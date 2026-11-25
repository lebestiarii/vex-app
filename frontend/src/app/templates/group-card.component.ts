import { ChangeDetectionStrategy, Component } from '@angular/core';
import { GroupLayoutData } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

/**
 * GroupCardComponent (`group`):
 * Strictly header-only container; renders zero leaf body.
 * Nested children are stacked full-width by RolodexCardShellComponent.
 */
@Component({
  selector: 'app-group-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: ``,
})
export class GroupCardComponent extends BaseCardTemplate<GroupLayoutData> {}
