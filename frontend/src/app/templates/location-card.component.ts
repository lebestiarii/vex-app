import { ChangeDetectionStrategy, Component } from '@angular/core';
import { BaseCardTemplate } from './base-card-template';

/**
 * LocationCardComponent (`template: 'location'`):
 * Strictly header-only entity card (no street addresses, phone numbers, or contact rows).
 * When expanded, RolodexCardShellComponent renders only its full-width stacked child cards.
 */
@Component({
  selector: 'app-location-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: ``,
})
export class LocationCardComponent extends BaseCardTemplate<Record<string, unknown>> {}
