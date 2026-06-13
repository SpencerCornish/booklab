import type { ReactNode } from 'react'

type Column<T> = {
  header: string
  headerClassName?: string
  cellClassName?: string
  hideBelow?: 'sm' | 'md'
  render: (item: T) => ReactNode
}

function hideClass(hideBelow?: 'sm' | 'md') {
  if (hideBelow === 'sm') return 'hidden sm:table-cell'
  if (hideBelow === 'md') return 'hidden md:table-cell'
  return ''
}

type Props<T> = {
  items: T[]
  keyFn: (item: T) => string | number
  columns: Column<T>[]
  renderCard: (item: T) => ReactNode
  emptyMessage?: string
  panelClassName?: string
  theadClassName?: string
  tableClassName?: string
  minTableWidth?: string
}

export function ResponsiveDataView<T>({
  items,
  keyFn,
  columns,
  renderCard,
  emptyMessage = 'No items.',
  panelClassName = 'bg-white rounded-xl border border-gray-200',
  theadClassName = 'bg-gray-50 border-b border-gray-200',
  tableClassName = 'w-full',
  minTableWidth,
}: Props<T>) {
  if (items.length === 0) {
    return <p className="text-sm text-gray-400 py-8 text-center">{emptyMessage}</p>
  }

  return (
    <>
      <div className="md:hidden space-y-3">
        {items.map((item) => (
          <div key={keyFn(item)}>{renderCard(item)}</div>
        ))}
      </div>

      <div className={`hidden md:block overflow-x-auto ${panelClassName}`}>
        <table
          className={tableClassName}
          style={minTableWidth ? { minWidth: minTableWidth } : undefined}
        >
          <thead className={theadClassName}>
            <tr>
              {columns.map((col) => (
                <th
                  key={col.header}
                  className={`px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase ${hideClass(col.hideBelow)} ${col.headerClassName ?? ''}`}
                >
                  {col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {items.map((item) => (
              <tr key={keyFn(item)} className="hover:bg-gray-50">
                {columns.map((col) => (
                  <td
                    key={col.header}
                    className={`px-4 py-3 ${hideClass(col.hideBelow)} ${col.cellClassName ?? ''}`}
                  >
                    {col.render(item)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}
